#!/usr/bin/env bash
# quickstart-check.sh — time-to-first-trace check for the one-command quickstart.
#
# From a clean machine (CI), runs deployments/quickstart/quickstart.sh and
# fails if the stack is not up with a demo-agent trace visible in the UI
# within the budget. Then sends a trace as my-agent with the generated certs,
# the way a new user would, and checks it shows up too.
#
#   QUICKSTART_BUDGET_SECONDS   default 300 (the activation goal: under 5 minutes)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
Q="$ROOT/deployments/quickstart"
UI="http://localhost:${CONDUIT_UI_PORT:-8080}"
BUDGET="${QUICKSTART_BUDGET_SECONDS:-300}"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

start=$(date +%s)
"$Q/quickstart.sh"
elapsed=$(( $(date +%s) - start ))
(( elapsed <= BUDGET )) || fail "quickstart took ${elapsed}s, over the ${BUDGET}s budget"
pass "quickstart up with a first trace in ${elapsed}s (budget ${BUDGET}s)"
[[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && echo "Time to first trace (cold quickstart): **${elapsed}s** (budget ${BUDGET}s)" >> "$GITHUB_STEP_SUMMARY"

echo "==> UI login with the generated credentials"
read -r user pass < "$Q/.conduit/ui-login.txt"
jar=$(mktemp)
trap 'rm -f "$jar"' EXIT
code=$(curl -s -c "$jar" -o /dev/null -w '%{http_code}' --data-urlencode "username=$user" --data-urlencode "password=$pass" -d next=/traces "$UI/login")
[[ "$code" == 303 ]] || fail "login returned $code, want 303"
curl -sf -b "$jar" "$UI/traces" | grep -q 'demo-agent' || fail "demo-agent not listed on /traces"
pass "logged in; demo-agent traces are listed"

echo "==> A trace from your own agent (my-agent)"
certs="$Q/.conduit/certs"
trace=$(go -C "$ROOT/sdk/go" run ./examples/smoke-agent -addr "localhost:${CONDUIT_GATEWAY_PORT:-4317}" \
  -cert "$certs/my-agent.crt" -key "$certs/my-agent.key" -ca "$certs/ca.crt")
[[ "$trace" =~ ^[0-9a-f]{32}$ ]] || fail "unexpected trace id: $trace"
for _ in $(seq 30); do
  if curl -sf -b "$jar" "$UI/traces/$trace" | grep -q 'my-agent'; then
    pass "trace $trace from my-agent is visible in the UI"
    exit 0
  fi
  sleep 1
done
fail "trace $trace from my-agent not visible at $UI/traces/$trace"
