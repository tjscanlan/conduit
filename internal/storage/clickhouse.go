package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// DDL for the spans table — run once on startup or via migration
const createSpansTable = `
CREATE TABLE IF NOT EXISTS spans (
    trace_id        String,
    span_id         String,
    parent_id       String,
    agent_name      String,        -- from mTLS CommonName
    span_name       String,
    start_time      DateTime64(9, 'UTC'),
    end_time        DateTime64(9, 'UTC'),
    duration_ms     UInt64,
    status          Enum8('ok'=0, 'error'=1, 'unset'=2),
    attributes      Map(String, String),
    -- LLM-specific fields (nullable — not all spans are LLM calls)
    llm_model       Nullable(String),
    llm_prompt      Nullable(String),
    llm_completion  Nullable(String),
    llm_input_tokens  Nullable(UInt32),
    llm_output_tokens Nullable(UInt32),
    llm_cost_usd    Nullable(Float64),
    -- Metadata
    ingested_at     DateTime DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(start_time)
ORDER BY (agent_name, trace_id, start_time)
TTL toDateTime(start_time) + INTERVAL 90 DAY;
`

// createTracesView keeps per-trace rollups. Columns are SimpleAggregateFunction
// (via -SimpleState) so AggregatingMergeTree merges rows from separate inserts
// correctly; plain min/max/sum columns would keep an arbitrary row on merge.
// Query with the matching aggregate and GROUP BY, since unmerged parts may
// coexist, and compute duration there:
//
//	SELECT trace_id, min(started_at), max(ended_at),
//	       dateDiff('millisecond', min(started_at), max(ended_at)) AS duration_ms,
//	       sum(span_count), sum(error_count), sum(total_cost_usd)
//	FROM trace_summary GROUP BY trace_id
const createTracesView = `
CREATE MATERIALIZED VIEW IF NOT EXISTS trace_summary
ENGINE = AggregatingMergeTree()
ORDER BY (trace_id)
AS SELECT
    trace_id,
    minSimpleState(start_time)                              AS started_at,
    maxSimpleState(end_time)                                AS ended_at,
    sumSimpleState(toUInt64(1))                             AS span_count,
    sumSimpleState(toUInt64(status = 'error'))              AS error_count,
    sumSimpleState(toUInt64(ifNull(llm_input_tokens, 0)))   AS total_input_tokens,
    sumSimpleState(toUInt64(ifNull(llm_output_tokens, 0)))  AS total_output_tokens,
    sumSimpleState(ifNull(llm_cost_usd, 0))                 AS total_cost_usd
FROM spans
GROUP BY trace_id;
`

type ClickHouseStore struct {
	conn driver.Conn
}

// Ready pings ClickHouse.
func (s *ClickHouseStore) Ready(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

func NewClickHouseStore(dsn string) (*ClickHouseStore, error) {
	opts, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	opts.TLS = nil // handled externally via mTLS at network layer

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := conn.Exec(ctx, createSpansTable); err != nil {
		return nil, err
	}
	if err := conn.Exec(ctx, createTracesView); err != nil {
		return nil, err
	}

	return &ClickHouseStore{conn: conn}, nil
}

type SpanRow struct {
	TraceID         string
	SpanID          string
	ParentID        string
	AgentName       string
	SpanName        string
	StartTime       time.Time
	EndTime         time.Time
	DurationMs      uint64
	Status          string
	Attributes      map[string]string
	LLMModel        *string
	LLMPrompt       *string
	LLMCompletion   *string
	LLMInputTokens  *uint32
	LLMOutputTokens *uint32
	LLMCostUSD      *float64
}

// InsertSpan writes one span. It uses the native batch protocol rather than
// Exec with ? placeholders: positional binding formats time.Time at second
// precision, which silently truncated every DateTime64(9) timestamp.
func (s *ClickHouseStore) InsertSpan(ctx context.Context, row SpanRow) error {
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO spans (
			trace_id, span_id, parent_id, agent_name, span_name,
			start_time, end_time, duration_ms, status, attributes,
			llm_model, llm_prompt, llm_completion,
			llm_input_tokens, llm_output_tokens, llm_cost_usd
		)`)
	if err != nil {
		return err
	}
	if err := batch.Append(
		row.TraceID, row.SpanID, row.ParentID, row.AgentName, row.SpanName,
		row.StartTime, row.EndTime, row.DurationMs, row.Status, row.Attributes,
		row.LLMModel, row.LLMPrompt, row.LLMCompletion,
		row.LLMInputTokens, row.LLMOutputTokens, row.LLMCostUSD,
	); err != nil {
		_ = batch.Abort()
		return err
	}
	return batch.Send()
}

// TraceSummary is one row of the trace list: a whole trace rolled up from its spans.
type TraceSummary struct {
	TraceID      string
	RootName     string // name of the root span (or earliest span if none is a root)
	Started      time.Time
	Ended        time.Time
	SpanCount    uint64
	ErrorCount   uint64
	Agents       []string
	InputTokens  uint64
	OutputTokens uint64
	CostUSD      float64
}

// TraceFilter selects traces for the list page.
type TraceFilter struct {
	Window     time.Duration // only traces with spans starting within this window (required, >0)
	Agent      string        // only traces that include a span from this agent
	ErrorsOnly bool          // only traces with at least one error span
	Limit      int

	// Keyset cursor: only traces strictly older than (BeforeStarted, BeforeTraceID)
	// in (started DESC, trace_id DESC) order. Zero BeforeStarted means the first page.
	BeforeStarted time.Time
	BeforeTraceID string
}

// QueryTraces returns trace summaries, newest first. The time window bounds
// the scan; it is passed as whole seconds rather than a time.Time, whose
// positional binding would be truncated to seconds anyway.
func (s *ClickHouseStore) QueryTraces(ctx context.Context, f TraceFilter) ([]TraceSummary, error) {
	window := int64(f.Window / time.Second)
	if window <= 0 || f.Limit <= 0 {
		return nil, fmt.Errorf("query traces: window and limit must be positive")
	}
	where := "start_time >= now64(9) - toIntervalSecond(?)"
	args := []any{window}
	if f.Agent != "" {
		// Select whole traces that include the agent, so totals still cover every span.
		where += " AND trace_id IN (SELECT trace_id FROM spans WHERE agent_name = ? AND start_time >= now64(9) - toIntervalSecond(?))"
		args = append(args, f.Agent, window)
	}
	var conds []string
	if f.ErrorsOnly {
		conds = append(conds, "error_count > 0")
	}
	if !f.BeforeStarted.IsZero() {
		// The cursor time travels as integer nanoseconds: binding a time.Time
		// would truncate it to seconds and skip or repeat traces at page edges.
		conds = append(conds, "(started, trace_id) < (fromUnixTimestamp64Nano(toInt64(?), 'UTC'), ?)")
		args = append(args, f.BeforeStarted.UnixNano(), f.BeforeTraceID)
	}
	having := ""
	if len(conds) > 0 {
		having = "HAVING " + strings.Join(conds, " AND ")
	}
	args = append(args, f.Limit)

	rows, err := s.conn.Query(ctx, `
		SELECT trace_id,
		       argMin(span_name, (parent_id != '', start_time)) AS root_name,
		       min(start_time)                                   AS started,
		       max(end_time)                                     AS ended,
		       count()                                           AS span_count,
		       countIf(status = 'error')                         AS error_count,
		       arraySort(groupUniqArray(20)(agent_name))         AS agents,
		       sum(toUInt64(ifNull(llm_input_tokens, 0)))        AS input_tokens,
		       sum(toUInt64(ifNull(llm_output_tokens, 0)))       AS output_tokens,
		       sum(ifNull(llm_cost_usd, 0))                      AS cost_usd
		FROM spans
		WHERE `+where+`
		GROUP BY trace_id
		`+having+`
		ORDER BY started DESC, trace_id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TraceSummary
	for rows.Next() {
		var t TraceSummary
		if err := rows.Scan(&t.TraceID, &t.RootName, &t.Started, &t.Ended, &t.SpanCount, &t.ErrorCount,
			&t.Agents, &t.InputTokens, &t.OutputTokens, &t.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// QueryAgents returns the distinct agent names seen within window, sorted,
// at most limit of them (for the trace list's agent suggestions).
func (s *ClickHouseStore) QueryAgents(ctx context.Context, window time.Duration, limit int) ([]string, error) {
	secs := int64(window / time.Second)
	if secs <= 0 || limit <= 0 {
		return nil, fmt.Errorf("query agents: window and limit must be positive")
	}
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT agent_name FROM spans
		WHERE start_time >= now64(9) - toIntervalSecond(?) AND agent_name != ''
		ORDER BY agent_name LIMIT ?`, secs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// QueryTrace returns up to limit spans of one trace, earliest first.
func (s *ClickHouseStore) QueryTrace(ctx context.Context, traceID string, limit int) ([]SpanRow, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT trace_id, span_id, parent_id, agent_name, span_name,
		        start_time, end_time, duration_ms, status, attributes,
		        llm_model, llm_input_tokens, llm_output_tokens, llm_cost_usd
		 FROM spans WHERE trace_id = ? ORDER BY start_time ASC LIMIT ?`, traceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SpanRow
	for rows.Next() {
		var r SpanRow
		if err := rows.Scan(
			&r.TraceID, &r.SpanID, &r.ParentID, &r.AgentName, &r.SpanName,
			&r.StartTime, &r.EndTime, &r.DurationMs, &r.Status, &r.Attributes,
			&r.LLMModel, &r.LLMInputTokens, &r.LLMOutputTokens, &r.LLMCostUSD,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
