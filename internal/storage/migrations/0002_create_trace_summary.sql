-- Per-trace rollups. SimpleAggregateFunction columns (via -SimpleState) merge
-- correctly across inserts; query with the matching aggregate + GROUP BY.
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
