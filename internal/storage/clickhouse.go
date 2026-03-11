package storage

import (
	"context"
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
TTL start_time + INTERVAL 90 DAY;
`

const createTracesView = `
CREATE MATERIALIZED VIEW IF NOT EXISTS trace_summary
ENGINE = AggregatingMergeTree()
ORDER BY (trace_id)
AS SELECT
    trace_id,
    min(start_time)         AS started_at,
    max(end_time)           AS ended_at,
    max(end_time) - min(start_time) AS total_duration_ms,
    countIf(status = 'error') AS error_count,
    sum(llm_input_tokens)   AS total_input_tokens,
    sum(llm_output_tokens)  AS total_output_tokens,
    sum(llm_cost_usd)       AS total_cost_usd
FROM spans
GROUP BY trace_id;
`

type ClickHouseStore struct {
	conn driver.Conn
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
	TraceID        string
	SpanID         string
	ParentID       string
	AgentName      string
	SpanName       string
	StartTime      time.Time
	EndTime        time.Time
	DurationMs     uint64
	Status         string
	Attributes     map[string]string
	LLMModel       *string
	LLMPrompt      *string
	LLMCompletion  *string
	LLMInputTokens *uint32
	LLMOutputTokens *uint32
	LLMCostUSD     *float64
}

func (s *ClickHouseStore) InsertSpan(ctx context.Context, row SpanRow) error {
	return s.conn.Exec(ctx, `
		INSERT INTO spans (
			trace_id, span_id, parent_id, agent_name, span_name,
			start_time, end_time, duration_ms, status, attributes,
			llm_model, llm_prompt, llm_completion,
			llm_input_tokens, llm_output_tokens, llm_cost_usd
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.TraceID, row.SpanID, row.ParentID, row.AgentName, row.SpanName,
		row.StartTime, row.EndTime, row.DurationMs, row.Status, row.Attributes,
		row.LLMModel, row.LLMPrompt, row.LLMCompletion,
		row.LLMInputTokens, row.LLMOutputTokens, row.LLMCostUSD,
	)
}

func (s *ClickHouseStore) QueryTrace(ctx context.Context, traceID string) ([]SpanRow, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT trace_id, span_id, parent_id, agent_name, span_name,
		        start_time, end_time, duration_ms, status, attributes,
		        llm_model, llm_input_tokens, llm_output_tokens, llm_cost_usd
		 FROM spans WHERE trace_id = ? ORDER BY start_time ASC`, traceID)
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
