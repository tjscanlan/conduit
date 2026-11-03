#!/usr/bin/env bash
# smoke-test.sh — end-to-end check of a running compose stack.
#
# Sends one trace with the Go SDK (agent -> gateway over mTLS -> NATS ->
# collector) and asserts it lands correctly in ClickHouse, VictoriaMetrics,
# and Tempo. Then it exercises the UI login flow.
#
# Prerequisites: the stack is up (docker compose up --wait), certs come from
# gen-certs.sh, and these env vars are set:
#   CLICKHOUSE_PASSWORD  the password the stack was started with
#   UI_USER, UI_PASSWORD credentials from deployments/docker/ui-users
#   SMOKE_PYTHON         python with sdk/python installed (default: python3);
#                        set SMOKE_SKIP_PYTHON=1 to skip the Python SDK check
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deployments/docker/docker-compose.yml")
UI=http://localhost:8080
VM=http://localhost:8428
: "${CLICKHOUSE_PASSWORD:?set CLICKHOUSE_PASSWORD}" "${UI_USER:?set UI_USER}" "${UI_PASSWORD:?set UI_PASSWORD}"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

# retry N CMD...: run CMD up to N times, 2s apart, until it succeeds.
retry() {
  local n=$1; shift
  for ((i = 1; i <= n; i++)); do
    if "$@"; then return 0; fi
    sleep 2
  done
  return 1
}

ch() {
  "${COMPOSE[@]}" exec -T -e CH_PW="$CLICKHOUSE_PASSWORD" clickhouse sh -c \
    'exec clickhouse-client --user obs --password "$CH_PW" --database observability -q "$1"' -- "$1" </dev/null
}

# The gateway must record the agent's identity from its certificate CN.
AGENT=$(openssl x509 -in "$ROOT/certs/agent-example.crt" -noout -subject | sed -n 's/.*CN *= *\([^,/]*\).*/\1/p')
[[ -n "$AGENT" ]] || fail "could not read CN from certs/agent-example.crt"

echo "==> Sending a trace through the gateway as $AGENT"
TRACE=$(cd "$ROOT/sdk/go" && go run ./examples/smoke-agent \
  -cert "$ROOT/certs/agent-example.crt" -key "$ROOT/certs/agent-example.key" -ca "$ROOT/certs/ca.crt")
[[ "$TRACE" =~ ^[0-9a-f]{32}$ ]] || fail "unexpected trace id: $TRACE"
pass "gateway accepted trace $TRACE"

echo "==> ClickHouse"
two_rows() { [[ "$(ch "SELECT count() FROM spans WHERE trace_id = '$TRACE'")" == 2 ]]; }
retry 30 two_rows || fail "expected 2 spans in ClickHouse for $TRACE"
rows=$(ch "SELECT span_name, agent_name, status, attributes['error.message'], ifNull(llm_model, ''), length(span_id), parent_id != ''
           FROM spans WHERE trace_id = '$TRACE' ORDER BY span_name FORMAT TSV")
expected="smoke.agent.run"$'\t'"$AGENT"$'\tok\t\t\t16\t0\n'"smoke.llm.call"$'\t'"$AGENT"$'\terror\ttool call timed out\tsmoke-model\t16\t1'
[[ "$rows" == "$expected" ]] || fail "span rows differ:"$'\n'"got:"$'\n'"$rows"$'\n'"want:"$'\n'"$expected"
pass "spans stored with agent_name from the cert CN, status, error.message, LLM fields, parent link"

# Timestamps must keep sub-second precision (the child span sleeps ~20 ms).
llm_ms=$(ch "SELECT dateDiff('millisecond', start_time, end_time) FROM spans WHERE trace_id = '$TRACE' AND span_name = 'smoke.llm.call'")
(( llm_ms >= 15 )) || fail "smoke.llm.call stored start/end only $llm_ms ms apart; timestamps lost sub-second precision"
pass "sub-second timestamps preserved ($llm_ms ms)"

summary=$(ch "SELECT sum(span_count), sum(error_count), sum(total_input_tokens) FROM trace_summary WHERE trace_id = '$TRACE' FORMAT TSV")
[[ "$summary" == $'2\t1\t12' ]] || fail "trace_summary = '$summary', want 2 spans / 1 error / 12 tokens"
pass "trace_summary rollup"

echo "==> Python SDK"
if [[ "${SMOKE_SKIP_PYTHON:-}" == 1 ]]; then
  echo "skip Python SDK (SMOKE_SKIP_PYTHON=1)"
else
  PY="${SMOKE_PYTHON:-python3}"
  "$PY" -c 'import conduit_sdk' 2>/dev/null || fail "conduit_sdk not importable with $PY (pip install -e sdk/python, set SMOKE_PYTHON, or SMOKE_SKIP_PYTHON=1)"
  PYTRACE=$("$PY" "$ROOT/sdk/python/examples/smoke_agent.py" \
    --cert "$ROOT/certs/agent-example.crt" --key "$ROOT/certs/agent-example.key" --ca "$ROOT/certs/ca.crt")
  [[ "$PYTRACE" =~ ^[0-9a-f]{32}$ ]] || fail "unexpected Python trace id: $PYTRACE"
  py_rows() { [[ "$(ch "SELECT count() FROM spans WHERE trace_id = '$PYTRACE'")" == 2 ]]; }
  retry 30 py_rows || fail "expected 2 Python SDK spans in ClickHouse for $PYTRACE"
  rows=$(ch "SELECT span_name, agent_name, status, attributes['error.message'], ifNull(llm_model, ''), length(span_id), parent_id != ''
             FROM spans WHERE trace_id = '$PYTRACE' ORDER BY span_name FORMAT TSV")
  expected="pysmoke.agent.run"$'\t'"$AGENT"$'\tok\t\t\t16\t0\n'"pysmoke.tool.call"$'\t'"$AGENT"$'\terror\ttool call timed out\tpy-model\t16\t1'
  [[ "$rows" == "$expected" ]] || fail "Python span rows differ:"$'\n'"got:"$'\n'"$rows"$'\n'"want:"$'\n'"$expected"
  pass "Python SDK trace $PYTRACE: context-manager spans, exception recorded as error, parent link"
fi

echo "==> VictoriaMetrics"
vm_has() {
  curl -fsS -G "$VM/api/v1/export" --data-urlencode 'match[]={__name__="conduit_span_duration_ms",span="smoke.llm.call",agent="'"$AGENT"'"}' | grep -q conduit_span_duration_ms
}
retry 30 vm_has || fail "no conduit_span_duration_ms series in VictoriaMetrics"
pass "span duration metrics"

echo "==> Tempo"
tempo_trace() {
  "${COMPOSE[@]}" exec -T tempo wget -qO- "http://127.0.0.1:3200/api/traces/$TRACE" </dev/null 2>/dev/null
}
tempo_ok() {
  local body; body=$(tempo_trace) || return 1
  grep -q STATUS_CODE_ERROR <<<"$body" && grep -q STATUS_CODE_OK <<<"$body" && grep -q smoke.llm.call <<<"$body"
}
retry 30 tempo_ok || fail "trace $TRACE not in Tempo with OK and ERROR statuses"
pass "trace in Tempo with statuses"

echo "==> UI"
jar=$(mktemp); trap 'rm -f "$jar"' EXIT
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
[[ "$(curl -fsS "$UI/healthz")" == ok ]] || fail "/healthz"
[[ "$(code "$UI/traces")" == 303 ]] || fail "unauthenticated /traces should redirect to login"
[[ "$(code -H "Origin: $UI" --data-urlencode "username=$UI_USER" --data-urlencode password=wrong "$UI/login")" == 401 ]] || fail "bad password should be 401"
[[ "$(code -H 'Origin: https://evil.example' --data-urlencode "username=$UI_USER" --data-urlencode "password=$UI_PASSWORD" "$UI/login")" == 403 ]] || fail "cross-site login should be 403"
[[ "$(code -c "$jar" -H "Origin: $UI" --data-urlencode "username=$UI_USER" --data-urlencode "password=$UI_PASSWORD" "$UI/login")" == 303 ]] || fail "login"
page=$(curl -fsS -b "$jar" "$UI/traces/$TRACE")
grep -q smoke.llm.call <<<"$page" && grep -q badge-error <<<"$page" || fail "trace detail page missing spans"
curl -fsSI -b "$jar" "$UI/traces" | grep -qi '^content-security-policy:' || fail "missing CSP header"
pass "login flow, trace detail page, CSP"

echo "PASS: smoke test for trace $TRACE"
