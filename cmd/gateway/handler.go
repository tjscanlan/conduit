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
	"github.com/your-org/agent-observability/internal/storage"
	"github.com/your-org/agent-observability/internal/telemetry"
	pb "github.com/your-org/agent-observability/proto"
)

// IngestionHandler implements pb.IngestionServiceServer for inbound spans from agent SDKs.
type IngestionHandler struct {
	pb.UnimplementedIngestionServiceServer

	publisher *storage.NATSPublisher
	logger    *slog.Logger
}

func NewIngestionHandler(cfg *Config, logger *slog.Logger) (*IngestionHandler, error) {
	pub, err := storage.NewNATSPublisher(cfg.NATSUrl)
	if err != nil {
		return nil, err
	}
	return &IngestionHandler{publisher: pub, logger: logger}, nil
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
	if n := len(req.GetLogs()); n > 0 {
		resp.Errors = append(resp.Errors, fmt.Sprintf("%d logs rejected: log ingestion not supported yet", n))
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

// ingestOne validates a SpanInput and publishes it to NATS as JSON.
// After gen-proto.sh is run this can be switched to proto.Marshal for efficiency.
func (h *IngestionHandler) ingestOne(ctx context.Context, in telemetry.SpanInput) (bool, string) {
	if id, ok := auth.IdentityFromContext(ctx); ok {
		in.AgentName = id.CommonName
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

// rateLimitInterceptor is a token-bucket rate limiter keyed on mTLS CommonName.
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

		v, _ := buckets.LoadOrStore(key, &bucket{tokens: float64(rps), lastSeen: time.Now()})
		b := v.(*bucket)

		b.mu.Lock()
		now := time.Now()
		b.tokens += now.Sub(b.lastSeen).Seconds() * float64(rps)
		if b.tokens > float64(rps) {
			b.tokens = float64(rps)
		}
		b.lastSeen = now
		if b.tokens < 1 {
			b.mu.Unlock()
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded for %q", key)
		}
		b.tokens--
		b.mu.Unlock()

		return handler(ctx, req)
	}
}

// authInterceptor delegates to the mTLS identity extractor in the auth package.
func authInterceptor() grpc.UnaryServerInterceptor {
	return auth.AuthInterceptor()
}
