package schema

import (
	"math"
	"strings"
	"testing"
	"time"
)

func validSpan() *SpanRequest {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &SpanRequest{
		TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:     "00f067aa0ba902b7",
		Name:       "agent.step",
		Status:     StatusOK,
		StartTime:  start,
		EndTime:    start.Add(time.Second),
		Attributes: map[string]string{"k": "v"},
		LLMCall:    &LLMCallData{Model: "claude-opus-5-5", InputTokens: 1, OutputTokens: 2, CostUSD: 0.01},
	}
}

func TestValidateAcceptsWellFormedSpan(t *testing.T) {
	if err := validSpan().Validate(); err != nil {
		t.Fatalf("valid span rejected: %v", err)
	}
	s := validSpan()
	s.TraceID = "123e4567-e89b-12d3-a456-426614174000" // UUID form
	s.ParentID = "a1b2c3d4e5f60718"
	if err := s.Validate(); err != nil {
		t.Fatalf("UUID trace id rejected: %v", err)
	}
}

func TestValidateAcceptsEveryStatus(t *testing.T) {
	for _, st := range []string{StatusUnset, StatusOK, StatusError} {
		s := validSpan()
		s.Status = st
		if err := s.Validate(); err != nil {
			t.Errorf("status %q rejected: %v", st, err)
		}
	}
	if got := New("t", "s", "", "n", time.Time{}, time.Time{}, nil).Status; got != StatusUnset {
		t.Errorf("New default status = %q, want %q", got, StatusUnset)
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SpanRequest)
	}{
		{"missing trace id", func(s *SpanRequest) { s.TraceID = "" }},
		{"oversized trace id", func(s *SpanRequest) { s.TraceID = strings.Repeat("a", MaxIDLen+1) }},
		{"trace id with control char", func(s *SpanRequest) { s.TraceID = "abc\n123" }},
		{"trace id with quote", func(s *SpanRequest) { s.TraceID = "abc'--" }},
		{"missing span id", func(s *SpanRequest) { s.SpanID = "" }},
		{"malformed span id", func(s *SpanRequest) { s.SpanID = "a b" }},
		{"malformed parent id", func(s *SpanRequest) { s.ParentID = "x;y" }},
		{"missing name", func(s *SpanRequest) { s.Name = "" }},
		{"empty status", func(s *SpanRequest) { s.Status = "" }},
		{"unknown status", func(s *SpanRequest) { s.Status = "SPAN_STATUS_FUTURE" }},
		{"oversized name", func(s *SpanRequest) { s.Name = strings.Repeat("n", MaxSpanNameLen+1) }},
		{"end before start", func(s *SpanRequest) { s.EndTime = s.StartTime.Add(-time.Second) }},
		{"too long duration", func(s *SpanRequest) { s.EndTime = s.StartTime.Add(MaxSpanDuration + time.Second) }},
		{"empty attribute key", func(s *SpanRequest) { s.Attributes[""] = "v" }},
		{"oversized attribute key", func(s *SpanRequest) { s.Attributes[strings.Repeat("k", MaxAttributeKeyLen+1)] = "v" }},
		{"oversized attribute value", func(s *SpanRequest) { s.Attributes["k"] = strings.Repeat("v", MaxAttributeValLen+1) }},
		{"too many attributes", func(s *SpanRequest) {
			for i := 0; i <= MaxAttributeCount; i++ {
				s.Attributes[strings.Repeat("k", i+1)] = "v"
			}
		}},
		{"oversized prompt", func(s *SpanRequest) { s.LLMCall.Prompt = strings.Repeat("p", MaxPromptBytes+1) }},
		{"oversized completion", func(s *SpanRequest) { s.LLMCall.Completion = strings.Repeat("c", MaxCompletionBytes+1) }},
		{"negative tokens", func(s *SpanRequest) { s.LLMCall.InputTokens = -1 }},
		{"oversized model", func(s *SpanRequest) { s.LLMCall.Model = strings.Repeat("m", MaxModelNameLen+1) }},
		{"NaN cost", func(s *SpanRequest) { s.LLMCall.CostUSD = math.NaN() }},
		{"infinite cost", func(s *SpanRequest) { s.LLMCall.CostUSD = math.Inf(1) }},
		{"negative cost", func(s *SpanRequest) { s.LLMCall.CostUSD = -0.01 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSpan()
			tt.mutate(s)
			if err := s.Validate(); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}
