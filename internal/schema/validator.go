package schema

import (
	"errors"
	"time"
)

const (
	MaxSpanNameLen      = 256
	MaxAttributeCount   = 128
	MaxAttributeValLen  = 4096
	MaxPromptBytes      = 1 << 17 // 128KB
	MaxCompletionBytes  = 1 << 17 // 128KB
	MaxSpanDuration     = 24 * time.Hour
)

// SpanRequest is the validated, internal representation of an inbound span
type SpanRequest struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Name       string
	StartTime  time.Time
	EndTime    time.Time
	Attributes map[string]string
	LLMCall    *LLMCallData // nil if not an LLM span
}

type LLMCallData struct {
	Model      string
	Prompt     string
	Completion string
	InputTokens  int32
	OutputTokens int32
	CostUSD    float64
}

// Validate enforces size and shape constraints at the ingestion boundary.
// Reject here — before anything touches storage.
func (s *SpanRequest) Validate() error {
	if s.TraceID == "" {
		return errors.New("trace_id is required")
	}
	if s.SpanID == "" {
		return errors.New("span_id is required")
	}
	if len(s.Name) == 0 {
		return errors.New("span name is required")
	}
	if len(s.Name) > MaxSpanNameLen {
		return errors.New("span name exceeds max length")
	}
	if s.EndTime.Before(s.StartTime) {
		return errors.New("end_time must be after start_time")
	}
	if s.EndTime.Sub(s.StartTime) > MaxSpanDuration {
		return errors.New("span duration exceeds maximum allowed")
	}
	if len(s.Attributes) > MaxAttributeCount {
		return errors.New("too many attributes")
	}
	for k, v := range s.Attributes {
		if len(k) > 128 {
			return errors.New("attribute key too long")
		}
		if len(v) > MaxAttributeValLen {
			return errors.New("attribute value too long")
		}
	}
	if s.LLMCall != nil {
		if err := s.LLMCall.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (l *LLMCallData) validate() error {
	if len(l.Prompt) > MaxPromptBytes {
		return errors.New("prompt exceeds max size")
	}
	if len(l.Completion) > MaxCompletionBytes {
		return errors.New("completion exceeds max size")
	}
	if l.InputTokens < 0 || l.OutputTokens < 0 {
		return errors.New("token counts must be non-negative")
	}
	return nil
}
