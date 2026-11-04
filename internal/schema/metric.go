package schema

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
)

// Metric types. Empty input is normalized to MetricUnspecified.
const (
	MetricUnspecified = "unspecified"
	MetricGauge       = "gauge"
	MetricCounter     = "counter"
)

const (
	MaxMetricNameLen       = 200
	MaxMetricLabels        = 32
	MaxMetricLabelKeyLen   = 128
	MaxMetricLabelValueLen = 1024
	// ReservedMetricPrefix is for Conduit's own metrics (e.g.
	// conduit_span_duration_ms); agents can't write series that look like them.
	ReservedMetricPrefix = "conduit_"
	// AgentLabel is set by the gateway from the mTLS identity, never by clients.
	AgentLabel = "agent"
)

var (
	metricNameRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelKeyRe   = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// MetricRequest is the validated, internal representation of a metric sample.
type MetricRequest struct {
	Name      string
	Labels    map[string]string // includes AgentLabel
	Value     float64
	Type      string
	Timestamp time.Time
}

// Validate enforces Prometheus naming rules and Conduit's limits.
func (m *MetricRequest) Validate(now time.Time) error {
	switch {
	case !metricNameRe.MatchString(m.Name) || len(m.Name) > MaxMetricNameLen:
		return errors.New("metric name must match [a-zA-Z_:][a-zA-Z0-9_:]* (max 200 chars)")
	case strings.HasPrefix(m.Name, ReservedMetricPrefix) || strings.HasPrefix(m.Name, "__"):
		return errors.New("metric names starting with conduit_ or __ are reserved")
	}
	switch m.Type {
	case MetricUnspecified, MetricGauge, MetricCounter:
	default:
		return errors.New("metric type must be unspecified, gauge, or counter")
	}
	if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
		return errors.New("metric value must be finite")
	}
	if m.Type == MetricCounter && m.Value < 0 {
		return errors.New("counter value must be non-negative")
	}
	if m.Timestamp.After(now.Add(MaxLogFutureSkew)) {
		return errors.New("metric timestamp is too far in the future")
	}
	if len(m.Labels) > MaxMetricLabels {
		return errors.New("too many labels")
	}
	for k, v := range m.Labels {
		if !labelKeyRe.MatchString(k) || len(k) > MaxMetricLabelKeyLen || strings.HasPrefix(k, "__") {
			return errors.New("label names must match [a-zA-Z_][a-zA-Z0-9_]* and not start with __")
		}
		if len(v) > MaxMetricLabelValueLen {
			return errors.New("label value too long")
		}
	}
	return nil
}
