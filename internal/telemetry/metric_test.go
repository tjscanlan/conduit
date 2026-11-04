package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/storage"
)

func TestProcessMetricAgentLabelAndDefaults(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	in := MetricInput{Name: "tokens_used", Value: 42, AgentName: "bot", Labels: map[string]string{"agent": "victim", "model": "m"}}
	req, err := ProcessMetric(in, now)
	if err != nil {
		t.Fatal(err)
	}
	if req.Labels["agent"] != "bot" || req.Labels["model"] != "m" || req.Type != "unspecified" || !req.Timestamp.Equal(now) {
		t.Errorf("req = %+v", req)
	}
	if in.Labels["agent"] != "victim" {
		t.Error("ProcessMetric must not mutate the caller's label map")
	}
}

type fakeMetricSink struct {
	err error
	got []storage.VMMetric
}

func (f *fakeMetricSink) WriteMetrics(_ context.Context, m []storage.VMMetric) error {
	f.got = append(f.got, m...)
	return f.err
}

func TestCollectorHandleMetricDispositions(t *testing.T) {
	good, _ := json.Marshal(MetricInput{Name: "q_depth", Value: 1, AgentName: "bot"})
	bad, _ := json.Marshal(MetricInput{Name: "conduit_fake", Value: 1, AgentName: "bot"})
	for _, tt := range []struct {
		name    string
		data    []byte
		sinkErr error
		want    disposition
		written int
	}{
		{"written", good, nil, ack, 1},
		{"sink failure retries (VictoriaMetrics is the system of record for metrics)", good, errors.New("vm down"), retry, 1},
		{"undecodable terminated", []byte("{"), nil, term, 0},
		{"invalid acked, not written", bad, nil, ack, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sink := &fakeMetricSink{err: tt.sinkErr}
			c := &Collector{metrics: sink, logger: discardLogger()}
			if got := c.handleMetric(context.Background(), tt.data); got != tt.want {
				t.Errorf("disposition = %v, want %v", got, tt.want)
			}
			if len(sink.got) != tt.written {
				t.Errorf("wrote %d, want %d", len(sink.got), tt.written)
			}
			if tt.written == 1 && sink.got[0].Labels["agent"] != "bot" {
				t.Errorf("written labels = %v", sink.got[0].Labels)
			}
		})
	}
}
