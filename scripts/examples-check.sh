#!/usr/bin/env bash
# examples-check.sh — run every example in examples/ against a running
# quickstart and check each trace shows up in the UI with the right spans.
#
# The Claude and OpenAI examples talk to scripts/fake-llm-server.py (both
# official SDKs honor ANTHROPIC_BASE_URL / OPENAI_BASE_URL), so this needs no
# API keys and exercises the examples' real tool loops and response parsing.
#
# Prerequisites: `make quickstart` is up; Go; a Python 3.10+ with
# sdk/python, anthropic and openai installed (EXAMPLES_PYTHON, default python3).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UI="http://localhost:${CONDUIT_UI_PORT:-8080}"
PY="${EXAMPLES_PYTHON:-python3}"
PORT="${FAKE_LLM_PORT:-18999}"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

"$PY" "$ROOT/scripts/fake-llm-server.py" "$PORT" 2>"${TMPDIR:-/tmp}/fake-llm.log" &
fake=$!
jar=$(mktemp)
trap 'kill $fake 2>/dev/null; wait $fake 2>/dev/null; rm -f "$jar"' EXIT
for _ in $(seq 20); do curl -s -o /dev/null "http://127.0.0.1:$PORT/" && break; sleep 0.25; done

read -r user pass < "$ROOT/deployments/quickstart/.conduit/ui-login.txt"
curl -s -c "$jar" -o /dev/null --data-urlencode "username=$user" --data-urlencode "password=$pass" "$UI/login"

export ANTHROPIC_API_KEY=fake ANTHROPIC_BASE_URL="http://127.0.0.1:$PORT"
export OPENAI_API_KEY=fake OPENAI_BASE_URL="http://127.0.0.1:$PORT/v1"

# has_all PAGE WORDS: every word appears in the page.
has_all() {
  local page=$1 w; shift
  for w in "$@"; do grep -q -- "$w" <<<"$page" || return 1; done
}

# check NAME "SPAN NAMES..." CMD...: run an example, then find its trace in the UI.
check() {
  local name=$1 spans=$2; shift 2
  local out trace page
  out=$("$@") || fail "$name exited $?: $out"
  trace=$(sed -n 's|.*/traces/\([0-9a-f]\{32\}\).*|\1|p' <<<"$out" | tail -1)
  [[ -n "$trace" ]] || fail "$name printed no trace link: $out"
  for _ in $(seq 30); do
    page=$(curl -sf -b "$jar" "$UI/traces/$trace" || true)
    # shellcheck disable=SC2086 # $spans is a word list
    if has_all "$page" my-agent $spans; then
      pass "$name -> trace $trace ($spans)"
      return
    fi
    sleep 1
  done
  fail "$name: trace $trace missing from the UI or lacks spans: $spans"
}

tool_spans="agent.run llm.call tool.get_weather"
cd "$ROOT/examples/go"
check "go hello-trace"   "hello.run hello.llm" go run ./hello-trace
check "go tool-agent"    "$tool_spans"         go run ./tool-agent
check "go claude-agent"  "$tool_spans"         go run ./claude-agent
check "go openai-agent"  "$tool_spans"         go run ./openai-agent
cd "$ROOT"
check "py hello_trace"   "hello.run hello.llm" "$PY" examples/python/hello_trace.py
check "py tool_agent"    "$tool_spans"         "$PY" examples/python/tool_agent.py
check "py async_agent"   "$tool_spans"         "$PY" examples/python/async_agent.py
check "py claude_agent"  "$tool_spans"         "$PY" examples/python/claude_agent.py
check "py openai_agent"  "$tool_spans"         "$PY" examples/python/openai_agent.py

# Without a key, the provider examples explain what to do instead of crashing.
msg=$(env -u ANTHROPIC_API_KEY "$PY" examples/python/claude_agent.py 2>&1 || true)
grep -q "Set ANTHROPIC_API_KEY" <<<"$msg" || fail "claude_agent.py without a key: $msg"
pass "provider examples explain the missing API key"
