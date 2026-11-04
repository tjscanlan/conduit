package telemetry

import (
	"fmt"
	"time"

	"github.com/tjscanlan/conduit/internal/schema"
)

// LogInput is a log record as published to NATS by the gateway.
type LogInput struct {
	TimestampNano int64
	Severity      string // schema.Severity*; empty means unspecified
	Body          string
	Attributes    map[string]string
	TraceID       string
	SpanID        string
	AgentName     string // set by the gateway from the mTLS CommonName
}

// ProcessLog validates a LogInput and returns a schema.LogRequest ready to store.
func ProcessLog(in LogInput, now time.Time) (*schema.LogRequest, error) {
	req := &schema.LogRequest{
		Timestamp:  nanoToTime(in.TimestampNano),
		AgentName:  in.AgentName,
		Severity:   in.Severity,
		Body:       in.Body,
		Attributes: in.Attributes,
		TraceID:    in.TraceID,
		SpanID:     in.SpanID,
	}
	if req.Severity == "" {
		req.Severity = schema.SeverityUnspecified
	}
	if req.Timestamp.IsZero() {
		req.Timestamp = now.UTC()
	}
	if req.Attributes == nil {
		req.Attributes = map[string]string{}
	}
	if err := req.Validate(now); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return req, nil
}
