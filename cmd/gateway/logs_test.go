package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/your-org/agent-observability/internal/auth"
	"github.com/your-org/agent-observability/internal/schema"
	"github.com/your-org/agent-observability/internal/telemetry"
	pb "github.com/your-org/agent-observability/proto"
)

func TestIngestBatchLogs(t *testing.T) {
	pub := &capturePublisher{}
	h := &IngestionHandler{publisher: pub, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), storePayloads: true}
	ctx := auth.WithIdentity(context.Background(), &auth.AgentIdentity{CommonName: "bot"})
	ts := timestamppb.New(time.Now())

	resp, err := h.IngestBatch(ctx, &pb.IngestBatchRequest{Logs: []*pb.LogRecord{
		{Timestamp: ts, Severity: pb.LogSeverity_LOG_SEVERITY_WARN, Body: "retrying tool call",
			TraceId: "4bf92f3577b34da6a3ce929d0e0e4736", SpanId: "00f067aa0ba902b7",
			Attributes: map[string]string{"agent_name": "spoofed", "tool": "search"}},
		{Timestamp: ts, Severity: pb.LogSeverity_LOG_SEVERITY_INFO, Body: ""},                 // no body
		{Timestamp: ts, Severity: pb.LogSeverity(42), Body: "x"},                              // unknown enum
		{Timestamp: ts, Body: strings.Repeat("b", schema.MaxLogBodyBytes+1)},                  // too big
		{Timestamp: timestamppb.New(time.Now().Add(48 * time.Hour)), Body: "from the future"}, // clock skew
		{Timestamp: ts, Body: "orphan span", SpanId: "00f067aa0ba902b7"},                      // span without trace
	}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetLogsAccepted() != 1 || len(resp.GetErrors()) != 5 {
		t.Fatalf("accepted %d, errors %v", resp.GetLogsAccepted(), resp.GetErrors())
	}
	for i, want := range []string{"log[1]: validation: log body is required", "log[2]: validation: severity", "log[3]: validation: log body exceeds", "log[4]: validation: log timestamp", "log[5]: validation: span_id requires trace_id"} {
		if !strings.HasPrefix(resp.GetErrors()[i], want) {
			t.Errorf("errors[%d] = %q, want prefix %q", i, resp.GetErrors()[i], want)
		}
	}
	if len(pub.logs) != 1 || len(pub.msgs) != 0 {
		t.Fatalf("published %d logs and %d spans; logs must go to the log subject", len(pub.logs), len(pub.msgs))
	}
	var in telemetry.LogInput
	if err := json.Unmarshal(pub.logs[0], &in); err != nil {
		t.Fatal(err)
	}
	if in.AgentName != "bot" || in.Severity != schema.SeverityWarn || in.Body != "retrying tool call" || in.TraceID == "" {
		t.Errorf("published log = %+v", in)
	}
}
