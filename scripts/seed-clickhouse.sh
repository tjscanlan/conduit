#!/usr/bin/env bash
# seed-clickhouse.sh — inserts sample spans for local dev / smoke testing
set -euo pipefail

HOST="${CLICKHOUSE_HOST:-localhost}"
PORT="${CLICKHOUSE_PORT:-9000}"
USER="${CLICKHOUSE_USER:-obs}"
PASS="${CLICKHOUSE_PASSWORD:-secret}"
DB="${CLICKHOUSE_DB:-observability}"

clickhouse-client \
  --host="$HOST" --port="$PORT" \
  --user="$USER" --password="$PASS" \
  --database="$DB" \
  --query="
INSERT INTO spans (
  trace_id, span_id, parent_id, agent_name, span_name,
  start_time, end_time, duration_ms, status, attributes,
  llm_model, llm_prompt, llm_completion,
  llm_input_tokens, llm_output_tokens, llm_cost_usd
) VALUES
  ('trace-seed-001', 'span-seed-001', '', 'example-agent', 'process-request',
   now() - INTERVAL 10 MINUTE, now() - INTERVAL 9 MINUTE, 60000, 'ok',
   map('env', 'dev', 'version', '1.0'),
   'claude-sonnet-4-6', 'Summarize this article.', 'Here is a concise summary...',
   512, 256, 0.0024),
  ('trace-seed-001', 'span-seed-002', 'span-seed-001', 'example-agent', 'fetch-context',
   now() - INTERVAL 10 MINUTE, now() - INTERVAL 9 MINUTE + INTERVAL 20 SECOND, 20000, 'ok',
   map('db.table', 'documents'),
   NULL, NULL, NULL, NULL, NULL, NULL),
  ('trace-seed-002', 'span-seed-003', '', 'example-agent', 'process-request',
   now() - INTERVAL 5 MINUTE, now() - INTERVAL 4 MINUTE, 60000, 'error',
   map('env', 'dev'),
   'claude-opus-4-6', 'Translate this text.', '',
   1024, 0, 0.0)
"

echo "Seed complete."
