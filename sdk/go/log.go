package sdk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/your-org/agent-observability/proto"
)

// Severity is a log record's level.
type Severity int32

const (
	SeverityUnspecified = Severity(pb.LogSeverity_LOG_SEVERITY_UNSPECIFIED)
	SeverityDebug       = Severity(pb.LogSeverity_LOG_SEVERITY_DEBUG)
	SeverityInfo        = Severity(pb.LogSeverity_LOG_SEVERITY_INFO)
	SeverityWarn        = Severity(pb.LogSeverity_LOG_SEVERITY_WARN)
	SeverityError       = Severity(pb.LogSeverity_LOG_SEVERITY_ERROR)
)

// Log sends one log record to the gateway. If ctx carries a span (from
// StartSpan), the record is linked to that span and its trace. attrs may be
// nil. It returns an error if the record isn't accepted.
func (c *Client) Log(ctx context.Context, severity Severity, body string, attrs map[string]string) error {
	rec := &pb.LogRecord{
		Timestamp:  timestamppb.New(time.Now()),
		Severity:   pb.LogSeverity(severity),
		Body:       body,
		Attributes: attrs,
	}
	if s, ok := ctx.Value(spanKey).(*Span); ok && s != nil {
		rec.TraceId, rec.SpanId = s.traceID, s.spanID
	}
	resp, err := c.ingest.IngestBatch(ctx, &pb.IngestBatchRequest{Logs: []*pb.LogRecord{rec}})
	if err != nil {
		return fmt.Errorf("ingest log: %w", err)
	}
	if resp.GetLogsAccepted() != 1 {
		if errs := resp.GetErrors(); len(errs) > 0 {
			return fmt.Errorf("gateway rejected log: %s", errs[0])
		}
		return errors.New("gateway rejected log")
	}
	return nil
}
