package schema

import "time"

// New builds a SpanRequest from raw field values.
// The gRPC handler extracts these from the incoming proto Span and calls this.
func New(traceID, spanID, parentID, name string, start, end time.Time, attrs map[string]string) *SpanRequest {
	if attrs == nil {
		attrs = make(map[string]string)
	}
	if start.IsZero() {
		start = time.Now()
	}
	if end.IsZero() {
		end = start
	}
	return &SpanRequest{
		TraceID:    traceID,
		SpanID:     spanID,
		ParentID:   parentID,
		Name:       name,
		StartTime:  start,
		EndTime:    end,
		Attributes: attrs,
	}
}
