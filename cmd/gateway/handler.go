package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/your-org/agent-observability/internal/auth"
	"github.com/your-org/agent-observability/internal/schema"
	"github.com/your-org/agent-observability/internal/storage"
	"github.com/your-org/agent-observability/internal/telemetry"
	pb "github.com/your-org/agent-observability/proto"
)

// IngestionHandler implements pb.IngestionServiceServer for inbound spans from agent SDKs.
type IngestionHandler struct {
	pb.UnimplementedIngestionServiceServer

	publisher spanPublisher
	logger    *slog.Logger

	// storePayloads keeps LLM prompts and completions. When false they are
	// dropped here, before anything is published, logged, or stored.
	storePayloads bool
}

// spanPublisher is the part of storage.NATSPublisher the handler uses.
type spanPublisher interface {
	Publish(ctx context.Context, data []byte) error
	PublishLog(ctx context.Context, data []byte) error
	Ready(ctx context.Context) error
}

func NewIngestionHandler(cfg *Config, logger *slog.Logger) (*IngestionHandler, error) {
	pub, err := storage.NewNATSPublisher(cfg.NATSUrl)
	if err != nil {
		return nil, err
	}
	if !cfg.StoreLLMPayloads {
		logger.Info("LLM prompt/completion storage disabled (STORE_LLM_PAYLOADS=false); payloads are dropped at the gateway")
	}
	return &IngestionHandler{publisher: pub, logger: logger, storePayloads: cfg.StoreLLMPayloads}, nil
}

// IngestSpan validates and publishes a single span.
func (h *IngestionHandler) IngestSpan(ctx context.Context, req *pb.IngestSpanRequest) (*pb.IngestSpanResponse, error) {
	if req.GetSpan() == nil {
		return nil, status.Error(codes.InvalidArgument, "span is required")
	}
	ok, reason := h.ingestOne(ctx, spanInputFromProto(req.GetSpan()))
	return &pb.IngestSpanResponse{Accepted: ok, RejectionReason: reason}, nil
}

// IngestBatch validates and publishes each span in the batch independently.
// Metrics and logs have no storage pipeline yet, so they are rejected.
func (h *IngestionHandler) IngestBatch(ctx context.Context, req *pb.IngestBatchRequest) (*pb.IngestBatchResponse, error) {
	resp := &pb.IngestBatchResponse{}
	for i, s := range req.GetSpans() {
		if ok, reason := h.ingestOne(ctx, spanInputFromProto(s)); ok {
			resp.SpansAccepted++
		} else {
			resp.Errors = append(resp.Errors, fmt.Sprintf("span[%d]: %s", i, reason))
		}
	}
	if n := len(req.GetMetrics()); n > 0 {
		resp.Errors = append(resp.Errors, fmt.Sprintf("%d metrics rejected: metric ingestion not supported yet", n))
	}
	for i, l := range req.GetLogs() {
		if ok, reason := h.ingestLog(ctx, logInputFromProto(l)); ok {
			resp.LogsAccepted++
		} else {
			resp.Errors = append(resp.Errors, fmt.Sprintf("log[%d]: %s", i, reason))
		}
	}
	return resp, nil
}

// spanInputFromProto flattens a pb.Span into the proto-free telemetry.SpanInput.
func spanInputFromProto(s *pb.Span) telemetry.SpanInput {
	in := telemetry.SpanInput{
		TraceID:    s.GetTraceId(),
		SpanID:     s.GetSpanId(),
		ParentID:   s.GetParentId(),
		Name:       s.GetName(),
		Status:     statusFromProto(s.GetStatus()),
		Attributes: s.GetAttributes(),
	}
	if t := s.GetStartTime(); t != nil {
		in.StartNano = t.AsTime().UnixNano()
	}
	if t := s.GetEndTime(); t != nil {
		in.EndNano = t.AsTime().UnixNano()
	}
	if llm := s.GetLlmCall(); llm != nil {
		in.HasLLMCall = true
		in.LLMModel = llm.GetModel()
		in.LLMPrompt = llm.GetPrompt()
		in.LLMCompletion = llm.GetCompletion()
		in.LLMInputTokens = llm.GetInputTokens()
		in.LLMOutputTokens = llm.GetOutputTokens()
		in.LLMCostUSD = llm.GetCostUsd()
	}
	return in
}

// statusFromProto maps the wire enum to schema status values. Unknown enum
// values (from a newer client) are passed through as invalid so validation
// rejects them instead of guessing.
func statusFromProto(st pb.SpanStatus) string {
	switch st {
	case pb.SpanStatus_SPAN_STATUS_UNSET:
		return schema.StatusUnset
	case pb.SpanStatus_SPAN_STATUS_OK:
		return schema.StatusOK
	case pb.SpanStatus_SPAN_STATUS_ERROR:
		return schema.StatusError
	}
	return st.String()
}

// severityFromProto maps the wire enum; unknown values from newer clients
// pass through as invalid so validation rejects them.
func severityFromProto(s pb.LogSeverity) string {
	switch s {
	case pb.LogSeverity_LOG_SEVERITY_UNSPECIFIED:
		return schema.SeverityUnspecified
	case pb.LogSeverity_LOG_SEVERITY_DEBUG:
		return schema.SeverityDebug
	case pb.LogSeverity_LOG_SEVERITY_INFO:
		return schema.SeverityInfo
	case pb.LogSeverity_LOG_SEVERITY_WARN:
		return schema.SeverityWarn
	case pb.LogSeverity_LOG_SEVERITY_ERROR:
		return schema.SeverityError
	}
	return s.String()
}

func logInputFromProto(l *pb.LogRecord) telemetry.LogInput {
	in := telemetry.LogInput{
		Severity:   severityFromProto(l.GetSeverity()),
		Body:       l.GetBody(),
		Attributes: l.GetAttributes(),
		TraceID:    l.GetTraceId(),
		SpanID:     l.GetSpanId(),
	}
	if t := l.GetTimestamp(); t != nil {
		in.TimestampNano = t.AsTime().UnixNano()
	}
	return in
}

// ingestLog validates a log record and publishes it to NATS.
func (h *IngestionHandler) ingestLog(ctx context.Context, in telemetry.LogInput) (bool, string) {
	if id, ok := auth.IdentityFromContext(ctx); ok {
		in.AgentName = id.CommonName // never client-supplied
	}
	if _, err := telemetry.ProcessLog(in, time.Now()); err != nil {
		return false, err.Error()
	}
	data, err := json.Marshal(in)
	if err != nil {
		return false, "internal marshal error"
	}
	if err := h.publisher.PublishLog(ctx, data); err != nil {
		h.logger.Error("nats publish log", "err", err)
		return false, "publish failed"
	}
	return true, ""
}

// ingestOne validates a SpanInput and publishes it to NATS as JSON.
// After gen-proto.sh is run this can be switched to proto.Marshal for efficiency.
func (h *IngestionHandler) ingestOne(ctx context.Context, in telemetry.SpanInput) (bool, string) {
	if id, ok := auth.IdentityFromContext(ctx); ok {
		in.AgentName = id.CommonName
	}
	if !h.storePayloads {
		// Before validation: an oversized prompt we're about to discard
		// shouldn't get the whole span rejected.
		in.LLMPrompt, in.LLMCompletion = "", ""
	}

	if _, err := telemetry.Process(in); err != nil {
		return false, err.Error()
	}

	data, err := json.Marshal(in)
	if err != nil {
		return false, "internal marshal error"
	}
	if err := h.publisher.Publish(ctx, data); err != nil {
		h.logger.Error("nats publish", "err", err)
		return false, "publish failed"
	}
	return true, ""
}

// unaryInterceptors returns the gateway's interceptor chain. Auth must run
// first: the rate limiter keys buckets on the mTLS identity it attaches.
func unaryInterceptors(rps int) []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		authInterceptor(),
		rateLimitInterceptor(rps),
	}
}

// maxBatchItems caps spans+metrics+logs in one IngestBatch call.
const maxBatchItems = 1000

// requestCost is how many rate-limit tokens a request consumes: one per
// telemetry item, so batching can't multiply an agent's allowed throughput.
func requestCost(req any) int {
	if b, ok := req.(*pb.IngestBatchRequest); ok {
		if n := len(b.GetSpans()) + len(b.GetMetrics()) + len(b.GetLogs()); n > 1 {
			return n
		}
	}
	return 1
}

// rateLimitInterceptor is a token-bucket rate limiter keyed on mTLS CommonName.
// Each agent may send rps telemetry items per second, with bursts up to rps.
func rateLimitInterceptor(rps int) grpc.UnaryServerInterceptor {
	type bucket struct {
		mu       sync.Mutex
		tokens   float64
		lastSeen time.Time
	}
	var buckets sync.Map

	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		key := ""
		if id, ok := auth.IdentityFromContext(ctx); ok {
			key = id.CommonName
		}

		cost := requestCost(req)
		if cost > maxBatchItems {
			return nil, status.Errorf(codes.InvalidArgument, "batch has %d items; max is %d", cost, maxBatchItems)
		}
		if cost > rps {
			// The bucket never holds more than rps tokens, so this could never succeed.
			return nil, status.Errorf(codes.InvalidArgument, "batch has %d items; per-agent rate limit is %d items/s, so split it", cost, rps)
		}

		v, _ := buckets.LoadOrStore(key, &bucket{tokens: float64(rps), lastSeen: time.Now()})
		b := v.(*bucket)

		b.mu.Lock()
		now := time.Now()
		b.tokens += now.Sub(b.lastSeen).Seconds() * float64(rps)
		if b.tokens > float64(rps) {
			b.tokens = float64(rps)
		}
		b.lastSeen = now
		if b.tokens < float64(cost) {
			b.mu.Unlock()
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded for %q", key)
		}
		b.tokens -= float64(cost)
		b.mu.Unlock()

		return handler(ctx, req)
	}
}

// authInterceptor delegates to the mTLS identity extractor in the auth package.
func authInterceptor() grpc.UnaryServerInterceptor {
	return auth.AuthInterceptor()
}
