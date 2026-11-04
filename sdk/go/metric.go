package sdk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/your-org/agent-observability/proto"
)

// Gauge records the current value of name (e.g. a queue depth).
func (c *Client) Gauge(ctx context.Context, name string, value float64, labels map[string]string) error {
	return c.metric(ctx, name, value, labels, pb.MetricType_METRIC_TYPE_GAUGE)
}

// Counter records a cumulative, non-negative value of name (e.g. requests_total).
func (c *Client) Counter(ctx context.Context, name string, value float64, labels map[string]string) error {
	return c.metric(ctx, name, value, labels, pb.MetricType_METRIC_TYPE_COUNTER)
}

// metric sends one sample. Names follow Prometheus rules; names starting with
// conduit_ are reserved, and the agent label is always set by the gateway.
func (c *Client) metric(ctx context.Context, name string, value float64, labels map[string]string, typ pb.MetricType) error {
	m := &pb.Metric{Name: name, Value: value, Labels: labels, Type: typ, Timestamp: timestamppb.New(time.Now())}
	resp, err := c.ingest.IngestBatch(ctx, &pb.IngestBatchRequest{Metrics: []*pb.Metric{m}})
	if err != nil {
		return fmt.Errorf("ingest metric: %w", err)
	}
	if resp.GetMetricsAccepted() != 1 {
		if errs := resp.GetErrors(); len(errs) > 0 {
			return fmt.Errorf("gateway rejected metric: %s", errs[0])
		}
		return errors.New("gateway rejected metric")
	}
	return nil
}
