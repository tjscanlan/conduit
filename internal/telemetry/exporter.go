package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tjscanlan/conduit/internal/schema"
	"github.com/tjscanlan/conduit/internal/storage"
)

// Backends the exporter writes to; satisfied by the storage package clients.
type (
	spanStore interface {
		InsertSpan(ctx context.Context, row storage.SpanRow) error
	}
	metricsWriter interface {
		WriteMetrics(ctx context.Context, metrics []storage.VMMetric) error
	}
	traceWriter interface {
		WriteSpan(ctx context.Context, s storage.TempoSpan) error
	}
)

// Exporter fans out a validated SpanRequest to all storage backends.
//
// ClickHouse is the system of record (the UI reads from it), so a failed
// insert is returned and the message is redelivered. VictoriaMetrics and
// Tempo are best-effort: their errors are logged, so a span already stored
// in ClickHouse is never redelivered and duplicated because of them.
type Exporter struct {
	ch     spanStore
	vm     metricsWriter
	tempo  traceWriter
	logger *slog.Logger
}

func NewExporter(
	ch *storage.ClickHouseStore,
	vm *storage.VictoriaMetricsClient,
	tempo *storage.TempoClient,
	logger *slog.Logger,
) *Exporter {
	return &Exporter{ch: ch, vm: vm, tempo: tempo, logger: logger}
}

func (e *Exporter) Export(ctx context.Context, req *schema.SpanRequest) error {
	agentName := req.Attributes["agent_name"]
	durationMs := uint64(req.EndTime.Sub(req.StartTime).Milliseconds())

	// ── ClickHouse: full span + LLM payload ──────────────────────────────
	row := storage.SpanRow{
		TraceID:    req.TraceID,
		SpanID:     req.SpanID,
		ParentID:   req.ParentID,
		AgentName:  agentName,
		SpanName:   req.Name,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		DurationMs: durationMs,
		Status:     req.Status,
		Attributes: req.Attributes,
	}
	if req.LLMCall != nil {
		row.LLMModel = &req.LLMCall.Model
		row.LLMPrompt = &req.LLMCall.Prompt
		row.LLMCompletion = &req.LLMCall.Completion
		it := uint32(req.LLMCall.InputTokens)
		ot := uint32(req.LLMCall.OutputTokens)
		row.LLMInputTokens = &it
		row.LLMOutputTokens = &ot
		row.LLMCostUSD = &req.LLMCall.CostUSD
	}
	if err := e.ch.InsertSpan(ctx, row); err != nil {
		return fmt.Errorf("clickhouse insert: %w", err)
	}

	// ── VictoriaMetrics: span duration gauge ─────────────────────────────
	metrics := []storage.VMMetric{{
		Name:      "conduit_span_duration_ms",
		Labels:    map[string]string{"agent": agentName, "span": req.Name},
		Value:     float64(durationMs),
		Timestamp: time.Now(),
	}}
	if err := e.vm.WriteMetrics(ctx, metrics); err != nil {
		e.logger.Error("victoriametrics write", "err", err)
	}

	// ── Tempo: trace data ────────────────────────────────────────────────
	// OTLP requires hex IDs of exact sizes; the gateway also accepts other
	// opaque IDs (e.g. UUIDs with dashes), which Tempo would reject.
	if !otlpIDs(req.TraceID, req.SpanID, req.ParentID) {
		e.logger.Debug("skipping tempo: ids are not OTLP-shaped", "trace_id", req.TraceID, "span_id", req.SpanID)
		return nil
	}
	if err := e.tempo.WriteSpan(ctx, storage.TempoSpan{
		TraceID:   req.TraceID,
		SpanID:    req.SpanID,
		ParentID:  req.ParentID,
		Name:      req.Name,
		Status:    req.Status,
		StartNano: req.StartTime.UnixNano(),
		EndNano:   req.EndTime.UnixNano(),
	}); err != nil {
		e.logger.Error("tempo write", "err", err)
	}

	return nil
}

// otlpIDs reports whether the IDs can be sent as OTLP: a 32-hex trace ID and
// 16-hex span and parent IDs (parent may be empty), not all zeros.
func otlpIDs(traceID, spanID, parentID string) bool {
	return isHexID(traceID, 32) && isHexID(spanID, 16) && (parentID == "" || isHexID(parentID, 16))
}

func isHexID(id string, n int) bool {
	if len(id) != n {
		return false
	}
	nonZero := false
	for i := 0; i < n; i++ {
		c := id[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
		if c != '0' {
			nonZero = true
		}
	}
	return nonZero
}
