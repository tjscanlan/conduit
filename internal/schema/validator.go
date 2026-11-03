package schema

import (
	"errors"
	"math"
	"time"
)

const (
	MaxSpanNameLen     = 256
	MaxAttributeCount  = 128
	MaxAttributeValLen = 4096
	MaxPromptBytes     = 1 << 17 // 128KB
	MaxCompletionBytes = 1 << 17 // 128KB
	MaxSpanDuration    = 24 * time.Hour
	MaxIDLen           = 64
	MaxAttributeKeyLen = 128
	MaxModelNameLen    = 256
)

// Span status values. They match the ClickHouse spans.status enum.
const (
	StatusUnset = "unset"
	StatusOK    = "ok"
	StatusError = "error"
)

// SpanRequest is the validated, internal representation of an inbound span
type SpanRequest struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Name       string
	Status     string // StatusUnset, StatusOK, or StatusError
	StartTime  time.Time
	EndTime    time.Time
	Attributes map[string]string
	LLMCall    *LLMCallData // nil if not an LLM span
}

type LLMCallData struct {
	Model        string
	Prompt       string
	Completion   string
	InputTokens  int32
	OutputTokens int32
	CostUSD      float64
}

// Validate enforces size and shape constraints at the ingestion boundary.
// Reject here — before anything touches storage.
func (s *SpanRequest) Validate() error {
	if s.TraceID == "" {
		return errors.New("trace_id is required")
	}
	if !validID(s.TraceID) {
		return errors.New("trace_id is malformed")
	}
	if s.SpanID == "" {
		return errors.New("span_id is required")
	}
	if !validID(s.SpanID) {
		return errors.New("span_id is malformed")
	}
	if s.ParentID != "" && !validID(s.ParentID) {
		return errors.New("parent_id is malformed")
	}
	if len(s.Name) == 0 {
		return errors.New("span name is required")
	}
	if len(s.Name) > MaxSpanNameLen {
		return errors.New("span name exceeds max length")
	}
	switch s.Status {
	case StatusUnset, StatusOK, StatusError:
	default:
		return errors.New("status must be unset, ok, or error")
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
		if k == "" {
			return errors.New("attribute key is empty")
		}
		if len(k) > MaxAttributeKeyLen {
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
	if len(l.Model) > MaxModelNameLen {
		return errors.New("model name exceeds max length")
	}
	if math.IsNaN(l.CostUSD) || math.IsInf(l.CostUSD, 0) || l.CostUSD < 0 {
		return errors.New("cost_usd must be a finite, non-negative number")
	}
	return nil
}

// validID reports whether id is a bounded, opaque identifier: ASCII letters,
// digits, '-' or '_'. This admits hex trace/span IDs and UUIDs while keeping
// control characters and query metacharacters out of storage.
func validID(id string) bool {
	if len(id) > MaxIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}
