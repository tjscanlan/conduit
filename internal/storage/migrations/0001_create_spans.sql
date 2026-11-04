-- Spans: the system of record. The 90-day TTL here is only the initial value;
-- the collector reconciles it to SPAN_RETENTION_DAYS after migrating.
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
