package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/your-org/agent-observability/internal/schema"
	"github.com/your-org/agent-observability/internal/storage"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func validInput() SpanInput {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return SpanInput{
		TraceID:   "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:    "00f067aa0ba902b7",
		Name:      "agent.step",
		StartNano: start.UnixNano(),
		EndNano:   start.Add(250 * time.Millisecond).UnixNano(),
		AgentName: "agent-1",
	}
}

func TestProcessStampsVerifiedAgentName(t *testing.T) {
	in := validInput()
	in.Attributes = map[string]string{"agent_name": "spoofed"}
	req, err := Process(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Attributes["agent_name"]; got != "agent-1" {
		t.Errorf("agent_name = %q, want the mTLS identity %q", got, "agent-1")
	}
	if req.LLMCall != nil {
		t.Error("LLMCall set without HasLLMCall")
	}
}

func TestProcessDefaultsAndValidation(t *testing.T) {
	in := validInput()
	in.StartNano, in.EndNano = 0, 0
	req, err := Process(in)
	if err != nil {
		t.Fatal(err)
	}
	if req.StartTime.IsZero() || !req.EndTime.Equal(req.StartTime) {
		t.Errorf("zero times should default to now/start: %v %v", req.StartTime, req.EndTime)
	}

	bad := validInput()
	bad.TraceID = ""
	if _, err := Process(bad); err == nil {
		t.Error("invalid span accepted")
	}
}

func TestProcessStatus(t *testing.T) {
	in := validInput()
	if req, _ := Process(in); req.Status != schema.StatusUnset {
		t.Errorf("empty status (e.g. a message from an older gateway) = %q, want unset", req.Status)
	}
	in.Status = schema.StatusError
	if req, _ := Process(in); req.Status != schema.StatusError {
		t.Errorf("status = %q, want error", req.Status)
	}
	in.Status = "bogus"
	if _, err := Process(in); err == nil {
		t.Error("unknown status accepted")
	}
}

func TestProcessMapsLLMCall(t *testing.T) {
	in := validInput()
	in.HasLLMCall, in.LLMModel, in.LLMInputTokens, in.LLMCostUSD = true, "m", 7, 0.5
	req, err := Process(in)
	if err != nil {
		t.Fatal(err)
	}
	if req.LLMCall == nil || req.LLMCall.Model != "m" || req.LLMCall.InputTokens != 7 || req.LLMCall.CostUSD != 0.5 {
		t.Errorf("LLMCall = %+v", req.LLMCall)
	}
}

type fakeExporter struct {
	err error
	got []*schema.SpanRequest
}

func (f *fakeExporter) Export(_ context.Context, req *schema.SpanRequest) error {
	f.got = append(f.got, req)
	return f.err
}

func TestCollectorHandleDispositions(t *testing.T) {
	good, _ := json.Marshal(validInput())
	invalidIn := validInput()
	invalidIn.SpanID = ""
	invalid, _ := json.Marshal(invalidIn)

	tests := []struct {
		name      string
		data      []byte
		exportErr error
		want      disposition
		exported  int
	}{
		{"success acks", good, nil, ack, 1},
		{"export failure retries", good, errors.New("clickhouse down"), retry, 1},
		{"undecodable is terminated, not redelivered", []byte("{not json"), nil, term, 0},
		{"invalid span is acked, not exported", invalid, nil, ack, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := &fakeExporter{err: tt.exportErr}
			c := &Collector{exporter: exp, logger: discardLogger()}
			if got := c.handle(context.Background(), tt.data); got != tt.want {
				t.Errorf("disposition = %v, want %v", got, tt.want)
			}
			if len(exp.got) != tt.exported {
				t.Errorf("exported %d spans, want %d", len(exp.got), tt.exported)
			}
		})
	}
}

type fakeStore struct {
	err  error
	rows []storage.SpanRow
}

func (f *fakeStore) InsertSpan(_ context.Context, row storage.SpanRow) error {
	f.rows = append(f.rows, row)
	return f.err
}

type fakeMetrics struct {
	err   error
	calls int
}

func (f *fakeMetrics) WriteMetrics(context.Context, []storage.VMMetric) error {
	f.calls++
	return f.err
}

type fakeTraces struct {
	err   error
	calls int
}

func (f *fakeTraces) WriteSpan(context.Context, storage.TempoSpan) error { f.calls++; return f.err }

func processed(t *testing.T) *schema.SpanRequest {
	t.Helper()
	in := validInput()
	in.HasLLMCall, in.LLMModel, in.LLMInputTokens, in.LLMOutputTokens = true, "m", 3, 4
	in.Status = schema.StatusError
	req, err := Process(in)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestExportReturnsClickHouseFailure(t *testing.T) {
	ch := &fakeStore{err: errors.New("connection refused")}
	vm, tempo := &fakeMetrics{}, &fakeTraces{}
	e := &Exporter{ch: ch, vm: vm, tempo: tempo, logger: discardLogger()}

	if err := e.Export(context.Background(), processed(t)); err == nil {
		t.Fatal("ClickHouse failure must be returned so the span is redelivered")
	}
	if vm.calls != 0 || tempo.calls != 0 {
		t.Error("secondary backends written even though the primary insert failed")
	}
}

func TestExportSecondaryBackendsAreBestEffort(t *testing.T) {
	ch := &fakeStore{}
	vm, tempo := &fakeMetrics{err: errors.New("vm down")}, &fakeTraces{err: errors.New("tempo down")}
	e := &Exporter{ch: ch, vm: vm, tempo: tempo, logger: discardLogger()}

	if err := e.Export(context.Background(), processed(t)); err != nil {
		t.Fatalf("secondary failures must not cause redelivery (would duplicate ClickHouse rows): %v", err)
	}
	if len(ch.rows) != 1 || vm.calls != 1 || tempo.calls != 1 {
		t.Fatalf("writes: ch=%d vm=%d tempo=%d", len(ch.rows), vm.calls, tempo.calls)
	}
	row := ch.rows[0]
	if row.Status != schema.StatusError {
		t.Errorf("row.Status = %q, want error (was hard-coded to ok)", row.Status)
	}
	if row.AgentName != "agent-1" || row.DurationMs != 250 || row.LLMModel == nil || *row.LLMModel != "m" ||
		row.LLMInputTokens == nil || *row.LLMInputTokens != 3 {
		t.Errorf("row = %+v", row)
	}
}
