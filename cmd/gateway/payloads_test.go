package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/your-org/agent-observability/internal/auth"
	"github.com/your-org/agent-observability/internal/schema"
	"github.com/your-org/agent-observability/internal/telemetry"
	pb "github.com/your-org/agent-observability/proto"
)

type capturePublisher struct{ msgs [][]byte }

func (c *capturePublisher) Publish(_ context.Context, data []byte) error {
	c.msgs = append(c.msgs, data)
	return nil
}
func (c *capturePublisher) Ready(context.Context) error { return nil }

func llmSpanRequest(prompt string) *pb.IngestSpanRequest {
	now := time.Now()
	return &pb.IngestSpanRequest{Span: &pb.Span{
		TraceId: "4bf92f3577b34da6a3ce929d0e0e4736", SpanId: "00f067aa0ba902b7", Name: "llm.call",
		StartTime: timestamppb.New(now), EndTime: timestamppb.New(now.Add(time.Second)),
		LlmCall: &pb.LLMCallData{Model: "m", Prompt: prompt, Completion: "secret completion",
			InputTokens: 10, OutputTokens: 20, CostUsd: 0.5},
	}}
}

func ingest(t *testing.T, store bool, req *pb.IngestSpanRequest) (*pb.IngestSpanResponse, *capturePublisher) {
	t.Helper()
	pub := &capturePublisher{}
	h := &IngestionHandler{publisher: pub, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), storePayloads: store}
	ctx := auth.WithIdentity(context.Background(), &auth.AgentIdentity{CommonName: "bot"})
	resp, err := h.IngestSpan(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, pub
}

func published(t *testing.T, pub *capturePublisher) telemetry.SpanInput {
	t.Helper()
	if len(pub.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(pub.msgs))
	}
	var in telemetry.SpanInput
	if err := json.Unmarshal(pub.msgs[0], &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestPayloadsDroppedWhenStorageDisabled(t *testing.T) {
	resp, pub := ingest(t, false, llmSpanRequest("secret prompt"))
	if !resp.GetAccepted() {
		t.Fatalf("rejected: %s", resp.GetRejectionReason())
	}
	raw := string(pub.msgs[0])
	if strings.Contains(raw, "secret prompt") || strings.Contains(raw, "secret completion") {
		t.Fatal("payload text left the gateway with STORE_LLM_PAYLOADS=false")
	}
	in := published(t, pub)
	if !in.HasLLMCall || in.LLMModel != "m" || in.LLMInputTokens != 10 || in.LLMOutputTokens != 20 || in.LLMCostUSD != 0.5 {
		t.Errorf("model, tokens, and cost must be kept: %+v", in)
	}
}

func TestOversizedPromptAcceptedWhenStorageDisabled(t *testing.T) {
	huge := strings.Repeat("p", schema.MaxPromptBytes+1)
	if resp, _ := ingest(t, true, llmSpanRequest(huge)); resp.GetAccepted() {
		t.Fatal("oversized prompt should be rejected when payloads are stored")
	}
	if resp, _ := ingest(t, false, llmSpanRequest(huge)); !resp.GetAccepted() {
		t.Fatalf("oversized prompt that will be discarded shouldn't reject the span: %s", resp.GetRejectionReason())
	}
}

func TestPayloadsKeptByDefault(t *testing.T) {
	_, pub := ingest(t, true, llmSpanRequest("kept prompt"))
	if in := published(t, pub); in.LLMPrompt != "kept prompt" || in.LLMCompletion != "secret completion" {
		t.Errorf("payloads should be kept: %+v", in)
	}
}

func TestStoreLLMPayloadsConfig(t *testing.T) {
	cases := map[string]struct {
		want    bool
		invalid bool
	}{
		"": {want: true}, "true": {want: true}, "1": {want: true}, "YES": {want: true}, "on": {want: true},
		"false": {want: false}, "0": {want: false}, "No": {want: false}, " off ": {want: false},
		"flase": {want: true, invalid: true}, "disabled": {want: true, invalid: true},
	}
	for v, tc := range cases {
		t.Setenv("STORE_LLM_PAYLOADS", v)
		if got := loadConfig().StoreLLMPayloads; got != tc.want {
			t.Errorf("STORE_LLM_PAYLOADS=%q -> %v, want %v", v, got, tc.want)
		}
		if err := validateConfig(); (err != nil) != tc.invalid {
			t.Errorf("STORE_LLM_PAYLOADS=%q: validateConfig err = %v, want invalid=%v", v, err, tc.invalid)
		}
	}
}
