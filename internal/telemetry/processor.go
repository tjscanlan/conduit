package telemetry

import (
	"fmt"
	"time"

	"github.com/your-org/agent-observability/internal/schema"
)

// SpanInput holds the raw field values extracted from an incoming proto Span
// by the gRPC handler. Using raw fields keeps this package proto-free until
// gen-proto.sh has been run.
type SpanInput struct {
	TraceID      string
	SpanID       string
	ParentID     string
	Name         string
	StartNano    int64
	EndNano      int64
	Attributes   map[string]string
	AgentName    string // set by handler from mTLS CommonName
	LLMModel     string
	LLMPrompt    string
	LLMCompletion string
	LLMInputTokens  int32
	LLMOutputTokens int32
	LLMCostUSD   float64
	HasLLMCall   bool
}

// Process validates a SpanInput and returns a schema.SpanRequest ready for export.
func Process(in SpanInput) (*schema.SpanRequest, error) {
	if in.Attributes == nil {
		in.Attributes = make(map[string]string)
	}
	if in.AgentName != "" {
		in.Attributes["agent_name"] = in.AgentName
	}

	start := nanoToTime(in.StartNano)
	end := nanoToTime(in.EndNano)

	req := schema.New(in.TraceID, in.SpanID, in.ParentID, in.Name, start, end, in.Attributes)

	if in.HasLLMCall {
		req.LLMCall = schema.NewLLMCall(
			in.LLMModel, in.LLMPrompt, in.LLMCompletion,
			in.LLMInputTokens, in.LLMOutputTokens, in.LLMCostUSD,
		)
	}

	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return req, nil
}

func nanoToTime(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}
