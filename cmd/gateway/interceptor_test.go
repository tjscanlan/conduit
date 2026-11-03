package main

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/your-org/agent-observability/internal/auth"
	pb "github.com/your-org/agent-observability/proto"
)

func okHandler(context.Context, any) (any, error) { return "ok", nil }

func callRL(t *testing.T, ic grpc.UnaryServerInterceptor, cn string) codes.Code {
	t.Helper()
	ctx := auth.WithIdentity(context.Background(), &auth.AgentIdentity{CommonName: cn})
	_, err := ic(ctx, nil, &grpc.UnaryServerInfo{}, okHandler)
	return status.Code(err)
}

func TestRateLimitIsPerAgent(t *testing.T) {
	ic := rateLimitInterceptor(2)

	for i := 0; i < 2; i++ {
		if c := callRL(t, ic, "agent-a"); c != codes.OK {
			t.Fatalf("agent-a call %d: got %v, want OK", i, c)
		}
	}
	if c := callRL(t, ic, "agent-a"); c != codes.ResourceExhausted {
		t.Fatalf("agent-a over limit: got %v, want ResourceExhausted", c)
	}
	// A different agent has its own bucket.
	if c := callRL(t, ic, "agent-b"); c != codes.OK {
		t.Fatalf("agent-b: got %v, want OK (buckets must be per agent)", c)
	}
}

func callBatch(ic grpc.UnaryServerInterceptor, cn string, spans int) codes.Code {
	ctx := auth.WithIdentity(context.Background(), &auth.AgentIdentity{CommonName: cn})
	req := &pb.IngestBatchRequest{Spans: make([]*pb.Span, spans)}
	_, err := ic(ctx, req, &grpc.UnaryServerInfo{}, okHandler)
	return status.Code(err)
}

func TestRateLimitChargesPerBatchItem(t *testing.T) {
	ic := rateLimitInterceptor(10)

	// A 6-item batch spends 6 of 10 tokens, so another 6 doesn't fit...
	if c := callBatch(ic, "agent-a", 6); c != codes.OK {
		t.Fatalf("first batch: %v", c)
	}
	if c := callBatch(ic, "agent-a", 6); c != codes.ResourceExhausted {
		t.Fatalf("second batch: got %v, want ResourceExhausted (batches must not bypass the limit)", c)
	}
	// ...but 4 single spans do.
	for i := 0; i < 4; i++ {
		if c := callRL(t, ic, "agent-a"); c != codes.OK {
			t.Fatalf("single span %d: %v", i, c)
		}
	}
	if c := callRL(t, ic, "agent-a"); c != codes.ResourceExhausted {
		t.Fatalf("bucket should be empty: %v", c)
	}
}

func TestRateLimitRejectsImpossibleBatches(t *testing.T) {
	// Larger than the bucket: can never succeed, so fail fast and permanently.
	if c := callBatch(rateLimitInterceptor(10), "agent-a", 11); c != codes.InvalidArgument {
		t.Errorf("batch > rps: got %v, want InvalidArgument", c)
	}
	// Larger than the absolute cap, even with a huge rate limit.
	if c := callBatch(rateLimitInterceptor(1_000_000), "agent-a", maxBatchItems+1); c != codes.InvalidArgument {
		t.Errorf("batch > max: got %v, want InvalidArgument", c)
	}
	// Empty batch costs one token like any request.
	if requestCost(&pb.IngestBatchRequest{}) != 1 || requestCost(&pb.IngestSpanRequest{}) != 1 {
		t.Error("empty batch and single span should cost 1")
	}
}

func TestRateLimitConfigRejectsNonPositive(t *testing.T) {
	for _, v := range []string{"0", "-5", "abc"} {
		t.Setenv("RATE_LIMIT_RPS", v)
		if got := loadConfig().RateLimit; got != 100 {
			t.Errorf("RATE_LIMIT_RPS=%q -> %d, want default 100", v, got)
		}
	}
	t.Setenv("RATE_LIMIT_RPS", "250")
	if got := loadConfig().RateLimit; got != 250 {
		t.Errorf("RATE_LIMIT_RPS=250 -> %d", got)
	}
}

// chain runs interceptors in order, as grpc.ChainUnaryInterceptor does.
func chain(ics []grpc.UnaryServerInterceptor, h grpc.UnaryHandler) grpc.UnaryHandler {
	for i := len(ics) - 1; i >= 0; i-- {
		ic, next := ics[i], h
		h = func(ctx context.Context, req any) (any, error) {
			return ic(ctx, req, &grpc.UnaryServerInfo{}, next)
		}
	}
	return h
}

func TestInterceptorChainAuthenticatesBeforeRateLimiting(t *testing.T) {
	// With rps=0 the limiter rejects everything; an unauthenticated request
	// must still fail with Unauthenticated, proving auth runs first.
	h := chain(unaryInterceptors(0), okHandler)
	_, err := h(context.Background(), nil)
	if c := status.Code(err); c != codes.Unauthenticated {
		t.Fatalf("got %v, want Unauthenticated", c)
	}
}
