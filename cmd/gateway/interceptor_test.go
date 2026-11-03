package main

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/your-org/agent-observability/internal/auth"
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
