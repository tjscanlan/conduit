package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/your-org/agent-observability/internal/auth"
	"github.com/your-org/agent-observability/internal/storage"
	"github.com/your-org/agent-observability/internal/telemetry"
)

// IngestionHandler handles inbound spans from agent SDKs.
//
// NOTE: Once gen-proto.sh has been run, embed pb.UnimplementedIngestionServiceServer
// and swap the stub types below for the generated pb.Span / pb.IngestSpan* types.
// The handler logic itself will not change.
type IngestionHandler struct {
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
