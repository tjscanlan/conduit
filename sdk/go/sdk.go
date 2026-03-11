// Package sdk provides instrumentation for AI agents to emit telemetry
// to the observability gateway over mTLS/gRPC.
package sdk

import (
	"context"
	"crypto/tls"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Client is the agent-side SDK. Create one per process.
type Client struct {
	conn      *grpc.ClientConn
	agentName string
}

// Config holds connection settings for the SDK.
// All fields map to environment variables for 12-factor compatibility.
type Config struct {
	GatewayAddr string // OTEL_GATEWAY_ADDR, e.g. "gateway:4317"
	AgentName   string // OTEL_AGENT_NAME — used as mTLS CommonName
	TLSCert     string // path to client cert
	TLSKey      string // path to client key
	TLSCACert   string // path to CA cert
}

// New creates an SDK client and establishes the mTLS connection to the gateway.
func New(cfg Config) (*Client, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
	if err != nil {
		return nil, err
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	creds := credentials.NewTLS(tlsCfg)
	conn, err := grpc.NewClient(cfg.GatewayAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}

	return &Client{conn: conn, agentName: cfg.AgentName}, nil
}

// Span represents a unit of work in the agent execution
type Span struct {
	client    *Client
	traceID   string
	spanID    string
	parentID  string
	name      string
	startTime time.Time
	attrs     map[string]string
	llmCall   *LLMCall
}

// StartSpan begins a new span. Call span.End() when the work is done.
func (c *Client) StartSpan(ctx context.Context, name string) (*Span, context.Context) {
	s := &Span{
		client:    c,
		traceID:   traceIDFromContext(ctx),
		spanID:    newID(),
		parentID:  spanIDFromContext(ctx),
		name:      name,
		startTime: time.Now(),
		attrs:     make(map[string]string),
	}
	return s, contextWithSpan(ctx, s)
}

// SetAttr adds a key-value attribute to the span.
func (s *Span) SetAttr(key, value string) *Span {
	s.attrs[key] = value
	return s
}

// RecordLLMCall attaches LLM-specific metadata to this span.
// Call this after your LLM call completes.
func (s *Span) RecordLLMCall(call LLMCall) *Span {
	s.llmCall = &call
	return s
}

// End closes the span and flushes it to the gateway.
func (s *Span) End(ctx context.Context) {
	_ = s.client.flush(ctx, s, time.Now())
}

// LLMCall holds metadata about a single LLM inference call.
type LLMCall struct {
	Model        string
	Prompt       string
	Completion   string
	InputTokens  int32
	OutputTokens int32
	CostUSD      float64
}

func (c *Client) flush(ctx context.Context, s *Span, endTime time.Time) error {
	// Build and send OTLP span over gRPC — implementation calls generated proto client
	return nil // stub
}

// Close shuts down the gRPC connection gracefully.
func (c *Client) Close() error {
	return c.conn.Close()
}

func newID() string {
	// generate a random 16-byte hex ID
	return "" // stub — use crypto/rand in real impl
}

type contextKey int

const (
	traceIDKey contextKey = iota
	spanIDKey
	spanKey
)

func traceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey).(string); ok {
		return id
	}
	return newID()
}

func spanIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(spanIDKey).(string); ok {
		return id
	}
	return ""
}

func contextWithSpan(ctx context.Context, s *Span) context.Context {
	ctx = context.WithValue(ctx, traceIDKey, s.traceID)
	ctx = context.WithValue(ctx, spanIDKey, s.spanID)
	return context.WithValue(ctx, spanKey, s)
}
