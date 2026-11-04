-- Log records sent by agents. The 90-day TTL is the initial value; the
-- collector reconciles it to SPAN_RETENTION_DAYS like the spans table.
CREATE TABLE IF NOT EXISTS logs (
    timestamp    DateTime64(9, 'UTC'),
    agent_name   String,        -- from mTLS CommonName
    severity     Enum8('unspecified'=0, 'debug'=1, 'info'=2, 'warn'=3, 'error'=4),
    body         String,
    attributes   Map(String, String),
    trace_id     String,        -- empty when the log isn't tied to a trace
    span_id      String,
    ingested_at  DateTime DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (agent_name, timestamp)
TTL toDateTime(timestamp) + INTERVAL 90 DAY;
