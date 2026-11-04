package telemetry

import (
	"fmt"
	"maps"
	"time"

	"github.com/tjscanlan/conduit/internal/schema"
)

// MetricInput is a metric sample as published to NATS by the gateway.
type MetricInput struct {
	Name          string
	Labels        map[string]string
	Value         float64
	Type          string // schema.Metric*; empty means unspecified
	TimestampNano int64
	AgentName     string // set by the gateway from the mTLS CommonName
}

// ProcessMetric validates a MetricInput. The agent label always comes from
// the verified identity: any client-supplied "agent" label is overwritten.
func ProcessMetric(in MetricInput, now time.Time) (*schema.MetricRequest, error) {
	labels := maps.Clone(in.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[schema.AgentLabel] = in.AgentName
	req := &schema.MetricRequest{Name: in.Name, Labels: labels, Value: in.Value, Type: in.Type, Timestamp: nanoToTime(in.TimestampNano)}
	if req.Type == "" {
		req.Type = schema.MetricUnspecified
	}
	if req.Timestamp.IsZero() {
		req.Timestamp = now.UTC()
	}
	if err := req.Validate(now); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return req, nil
}
