// Package sdk provides instrumentation for AI agents to emit telemetry
// to the observability gateway over mTLS/gRPC.
package sdk

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/tjscanlan/conduit/proto"
)

// Client is the agent-side SDK. Create one per process.
type Client struct {
	cfg Config
	// certFromOtherCA: the agent certificate doesn't chain to TLSCACert. That's
	// allowed (the gateway may trust another CA) but is the usual cause when
	// the gateway drops the connection, so diagnose() says so.
	certFromOtherCA bool
	conn            *grpc.ClientConn
	ingest          pb.IngestionServiceClient
	agentName       string
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
// The gateway's certificate is verified against TLSCACert only, not the system
// roots, so only the Conduit CA can vouch for the gateway.
func New(cfg Config) (*Client, error) {
	if cfg.GatewayAddr == "" {
		return nil, errors.New("conduit: GatewayAddr is empty (the local quickstart's is " + DefaultGatewayAddr + ")")
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
	if err != nil {
		hint := ""
		switch {
		case errors.Is(err, os.ErrNotExist):
			hint = " (create agent certificates with `make quickstart` or `scripts/gen-certs.sh agent NAME`)"
		case strings.Contains(err.Error(), "does not match"):
			hint = " (the key belongs to a different certificate)"
		}
		return nil, fmt.Errorf("load client certificate: %w%s", err, hint)
	}
	leaf := cert.Leaf
	if leaf == nil {
		if leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
			return nil, fmt.Errorf("parse client certificate %s: %w", cfg.TLSCert, err)
		}
	}
	if err := checkClientCert(cfg.TLSCert, leaf); err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(cfg.TLSCACert)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("parse CA certificate: no PEM certificates found")
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	conn, err := grpc.NewClient(cfg.GatewayAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	if err != nil {
		return nil, err
	}

	_, verifyErr := leaf.Verify(x509.VerifyOptions{Roots: caPool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	return &Client{
		cfg: cfg, certFromOtherCA: verifyErr != nil,
		conn: conn, ingest: pb.NewIngestionServiceClient(conn), agentName: cfg.AgentName,
	}, nil
}

// Span represents a unit of work in the agent execution
type Span struct {
	client    *Client
	traceID   string
	spanID    string
	parentID  string
	name      string
	status    pb.SpanStatus
	startTime time.Time
	attrs     map[string]string
	llmCall   *LLMCall
}

// StartSpan begins a new span. Call span.End() when the work is done.
func (c *Client) StartSpan(ctx context.Context, name string) (*Span, context.Context) {
	s := &Span{
		client:    c,
		traceID:   traceIDFromContext(ctx),
		spanID:    newSpanID(),
		parentID:  spanIDFromContext(ctx),
		name:      name,
		startTime: time.Now(),
		attrs:     make(map[string]string),
	}
	return s, contextWithSpan(ctx, s)
}

// TraceID returns the span's 32-hex-character trace ID, e.g. for log correlation.
func (s *Span) TraceID() string { return s.traceID }

// SpanID returns the span's 16-hex-character span ID.
func (s *Span) SpanID() string { return s.spanID }

// SetAttr adds a key-value attribute to the span.
func (s *Span) SetAttr(key, value string) *Span {
	s.attrs[key] = value
	return s
}

// SetOK marks the span as successful.
func (s *Span) SetOK() *Span {
	s.status = pb.SpanStatus_SPAN_STATUS_OK
	return s
}

// SetError marks the span as failed and, when err is non-nil, records its
// message in the "error.message" attribute. A nil err is a no-op, so
// `defer span.SetError(err)`-style call sites stay simple.
func (s *Span) SetError(err error) *Span {
	if err == nil {
		return s
	}
	s.status = pb.SpanStatus_SPAN_STATUS_ERROR
	s.attrs["error.message"] = truncateUTF8(err.Error(), maxAttrValueLen)
	return s
}

// maxAttrValueLen mirrors the gateway's schema.MaxAttributeValLen; longer
// values would get the whole span rejected.
const maxAttrValueLen = 4096

// truncateUTF8 cuts s to at most n bytes without splitting a rune
// (proto3 strings must be valid UTF-8).
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// RecordLLMCall attaches LLM-specific metadata to this span.
// Call this after your LLM call completes.
func (s *Span) RecordLLMCall(call LLMCall) *Span {
	s.llmCall = &call
	return s
}

// End closes the span and sends it to the gateway. It returns an error if
// the span could not be delivered or the gateway rejected it.
func (s *Span) End(ctx context.Context) error {
	return s.client.flush(ctx, s, time.Now())
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

// flush sends the span to the gateway over gRPC.
func (c *Client) flush(ctx context.Context, s *Span, endTime time.Time) error {
	resp, err := c.ingest.IngestSpan(ctx, &pb.IngestSpanRequest{Span: spanToProto(s, endTime)})
	if err != nil {
		return c.diagnose("ingest span", err)
	}
	if !resp.GetAccepted() {
		return fmt.Errorf("gateway rejected span: %s", resp.GetRejectionReason())
	}
	return nil
}

func spanToProto(s *Span, endTime time.Time) *pb.Span {
	out := &pb.Span{
		TraceId:    s.traceID,
		SpanId:     s.spanID,
		ParentId:   s.parentID,
		Name:       s.name,
		Status:     s.status,
		StartTime:  timestamppb.New(s.startTime),
		EndTime:    timestamppb.New(endTime),
		Attributes: s.attrs,
	}
	if l := s.llmCall; l != nil {
		out.LlmCall = &pb.LLMCallData{
			Model:        l.Model,
			Prompt:       l.Prompt,
			Completion:   l.Completion,
			InputTokens:  l.InputTokens,
			OutputTokens: l.OutputTokens,
			CostUsd:      l.CostUSD,
		}
	}
	return out
}

// Close shuts down the gRPC connection gracefully.
func (c *Client) Close() error {
	return c.conn.Close()
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
	return newTraceID()
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
