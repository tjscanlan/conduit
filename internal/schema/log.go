package schema

import (
	"errors"
	"time"
)

// Log severities. They match the ClickHouse logs.severity enum.
const (
	SeverityUnspecified = "unspecified"
	SeverityDebug       = "debug"
	SeverityInfo        = "info"
	SeverityWarn        = "warn"
	SeverityError       = "error"
)

const (
	MaxLogBodyBytes = 32 << 10 // 32 KiB
	// MaxLogFutureSkew rejects timestamps implausibly far in the future
	// (clock skew beyond this is a client bug, not a real event time).
	MaxLogFutureSkew = time.Hour
)

// LogRequest is the validated, internal representation of an inbound log record.
type LogRequest struct {
	Timestamp  time.Time
	AgentName  string
	Severity   string
	Body       string
	Attributes map[string]string
	TraceID    string // optional; links the log to a trace
	SpanID     string // optional
}

// Validate enforces size and shape constraints at the ingestion boundary.
func (l *LogRequest) Validate(now time.Time) error {
	switch l.Severity {
	case SeverityUnspecified, SeverityDebug, SeverityInfo, SeverityWarn, SeverityError:
	default:
		return errors.New("severity must be unspecified, debug, info, warn, or error")
	}
	if l.Body == "" {
		return errors.New("log body is required")
	}
	if len(l.Body) > MaxLogBodyBytes {
		return errors.New("log body exceeds max size")
	}
	if l.Timestamp.After(now.Add(MaxLogFutureSkew)) {
		return errors.New("log timestamp is too far in the future")
	}
	if l.TraceID != "" && !validID(l.TraceID) {
		return errors.New("trace_id is malformed")
	}
	if l.SpanID != "" && !validID(l.SpanID) {
		return errors.New("span_id is malformed")
	}
	if l.SpanID != "" && l.TraceID == "" {
		return errors.New("span_id requires trace_id")
	}
	return validateAttributes(l.Attributes)
}
