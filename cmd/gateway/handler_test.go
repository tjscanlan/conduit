package main

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tjscanlan/conduit/internal/schema"
	"github.com/tjscanlan/conduit/internal/telemetry"
	pb "github.com/tjscanlan/conduit/proto"
)

func TestSpanInputFromProto(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	end := start.Add(time.Second)
	in := spanInputFromProto(&pb.Span{
		TraceId:    "t1",
		SpanId:     "s1",
		ParentId:   "p1",
		Name:       "llm.call",
		StartTime:  timestamppb.New(start),
		EndTime:    timestamppb.New(end),
		Attributes: map[string]string{"k": "v"},
		LlmCall: &pb.LLMCallData{
			Model: "m", Prompt: "p", Completion: "c",
			InputTokens: 10, OutputTokens: 20, CostUsd: 0.5,
		},
	})

	if in.TraceID != "t1" || in.SpanID != "s1" || in.ParentID != "p1" || in.Name != "llm.call" {
		t.Errorf("ids/name not copied: %+v", in)
	}
	if in.StartNano != start.UnixNano() || in.EndNano != end.UnixNano() {
		t.Errorf("times = %d,%d; want %d,%d", in.StartNano, in.EndNano, start.UnixNano(), end.UnixNano())
	}
	if in.Attributes["k"] != "v" {
		t.Errorf("attributes not copied: %v", in.Attributes)
	}
	if !in.HasLLMCall || in.LLMModel != "m" || in.LLMInputTokens != 10 || in.LLMOutputTokens != 20 || in.LLMCostUSD != 0.5 {
		t.Errorf("llm call not copied: %+v", in)
	}
}

func TestStatusFromProto(t *testing.T) {
	tests := map[pb.SpanStatus]string{
		pb.SpanStatus_SPAN_STATUS_UNSET: schema.StatusUnset,
		pb.SpanStatus_SPAN_STATUS_OK:    schema.StatusOK,
		pb.SpanStatus_SPAN_STATUS_ERROR: schema.StatusError,
	}
	for in, want := range tests {
		if got := spanInputFromProto(&pb.Span{Status: in}).Status; got != want {
			t.Errorf("%v -> %q, want %q", in, got, want)
		}
	}
	// An enum value this gateway doesn't know must fail validation, not be guessed.
	in := spanInputFromProto(&pb.Span{TraceId: "t1", SpanId: "s1", Name: "n", Status: pb.SpanStatus(99)})
	if _, err := telemetry.Process(in); err == nil {
		t.Error("unknown status enum accepted")
	}
}

func TestSpanInputFromProtoMissingOptionalFields(t *testing.T) {
	in := spanInputFromProto(&pb.Span{TraceId: "t1", SpanId: "s1", Name: "n"})
	if in.StartNano != 0 || in.EndNano != 0 {
		t.Errorf("unset timestamps should be zero, got %d,%d", in.StartNano, in.EndNano)
	}
	if in.HasLLMCall {
		t.Error("HasLLMCall should be false without llm_call")
	}
}
