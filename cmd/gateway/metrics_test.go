package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/auth"
	"github.com/tjscanlan/conduit/internal/telemetry"
	pb "github.com/tjscanlan/conduit/proto"
)

func TestIngestBatchMetrics(t *testing.T) {
	pub := &capturePublisher{}
	h := &IngestionHandler{publisher: pub, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), storePayloads: true}
	ctx := auth.WithIdentity(context.Background(), &auth.AgentIdentity{CommonName: "bot"})
	many := map[string]string{}
	for i := 0; i < 33; i++ {
		many[strings.Repeat("k", i+1)] = "v"
	}
	cases := []struct {
		m    *pb.Metric
		want string // "" = accepted; otherwise an error substring
	}{
		{&pb.Metric{Name: "agent_queue_depth", Value: 3, Type: pb.MetricType_METRIC_TYPE_GAUGE, Labels: map[string]string{"queue": "tasks", "agent": "spoofed"}}, ""},
		{&pb.Metric{Name: "conduit_span_duration_ms", Value: 1}, "reserved"},
		{&pb.Metric{Name: "__name__", Value: 1}, "reserved"},
		{&pb.Metric{Name: "bad-name", Value: 1}, "metric name must match"},
		{&pb.Metric{Name: "ok_name", Value: math.NaN()}, "finite"},
		{&pb.Metric{Name: "ok_name", Value: math.Inf(1)}, "finite"},
		{&pb.Metric{Name: "requests_total", Value: -1, Type: pb.MetricType_METRIC_TYPE_COUNTER}, "non-negative"},
		{&pb.Metric{Name: "ok_name", Value: 1, Labels: map[string]string{"bad-key": "v"}}, "label names"},
		{&pb.Metric{Name: "ok_name", Value: 1, Labels: map[string]string{"__hidden": "v"}}, "label names"},
		{&pb.Metric{Name: "ok_name", Value: 1, Labels: many}, "too many labels"},
		{&pb.Metric{Name: "ok_name", Value: 1, Type: pb.MetricType(9)}, "metric type"},
	}
	req := &pb.IngestBatchRequest{}
	for _, c := range cases {
		req.Metrics = append(req.Metrics, c.m)
	}
	resp, err := h.IngestBatch(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetMetricsAccepted() != 1 || len(resp.GetErrors()) != len(cases)-1 {
		t.Fatalf("accepted %d, errors %v", resp.GetMetricsAccepted(), resp.GetErrors())
	}
	for i, e := range resp.GetErrors() {
		if want := cases[i+1].want; !strings.Contains(e, want) {
			t.Errorf("errors[%d] = %q, want it to mention %q", i, e, want)
		}
	}
	if len(pub.metrics) != 1 || len(pub.msgs)+len(pub.logs) != 0 {
		t.Fatalf("published metrics=%d spans=%d logs=%d", len(pub.metrics), len(pub.msgs), len(pub.logs))
	}
	var in telemetry.MetricInput
	_ = json.Unmarshal(pub.metrics[0], &in)
	if in.AgentName != "bot" || in.Name != "agent_queue_depth" || in.Value != 3 || in.Type != "gauge" {
		t.Errorf("published metric = %+v", in)
	}
	// The stored labels get the verified agent, not the client's "agent" label.
	req2, err := telemetry.ProcessMetric(in, timeNow())
	if err != nil || req2.Labels["agent"] != "bot" || req2.Labels["queue"] != "tasks" {
		t.Errorf("processed labels = %v, %v", req2.Labels, err)
	}
}

func timeNow() time.Time { return time.Now() }
