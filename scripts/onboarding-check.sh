#!/usr/bin/env bash
# onboarding-check.sh — walk the UI's Get started flow on a fresh quickstart.
#
# Expects a freshly reset quickstart built from this checkout (the released
# images may predate the page):
#   deployments/quickstart/quickstart.sh reset
#   CONDUIT_FROM_SOURCE=1 deployments/quickstart/quickstart.sh
# Then: a new install lands on /start, which waits for the user's agent while
# demo-agent data flows; sending a trace as my-agent completes it; / then
# goes to /traces; and Get started can be dismissed.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UI="http://localhost:${CONDUIT_UI_PORT:-8080}"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

jar=$(mktemp)
trap 'rm -f "$jar"' EXIT
read -r user pass < "$ROOT/deployments/quickstart/.conduit/ui-login.txt"
curl -s -c "$jar" -o /dev/null --data-urlencode "username=$user" --data-urlencode "password=$pass" "$UI/login"
page() { curl -sf -b "$jar" -c "$jar" "$UI$1"; }
landing() { curl -s -b "$jar" -o /dev/null -w '%{redirect_url}' "$UI/"; }

[[ "$(landing)" == "$UI/start" ]] || fail "a new install should land on /start, got $(landing)"
start=$(page /start)
for want in "Get started" "is sending runs" "Waiting for a trace from your agent" 'hx-get="/start/status"' "gen-certs.sh agent"; do
  grep -qF "$want" <<<"$start" || fail "/start is missing: $want"
done
! grep -q "PRIVATE KEY" <<<"$start" || fail "/start must never contain key material"
pass "new install lands on /start: Conduit running, demo-agent flowing, waiting for your agent"

echo "==> Sending a first trace as my-agent"
export CONDUIT_CERT_DIR="$ROOT/deployments/quickstart/.conduit/certs" CONDUIT_AGENT=my-agent
trace=$(cd "$ROOT/examples/go" && go run ./hello-trace | sed -n 's/^Sent trace //p')
[[ "$trace" =~ ^[0-9a-f]{32}$ ]] || fail "hello-trace printed no trace id"
for _ in $(seq 30); do
  status=$(page /start/status)
  if grep -q "Your agent is connected" <<<"$status"; then
    grep -q "href=\"/traces/$trace\"" <<<"$status" || fail "status links the wrong trace: $status"
    ! grep -q "hx-trigger" <<<"$status" || fail "status should stop polling once connected"
    pass "Get started saw trace $trace from my-agent and stopped polling"
    break
  fi
  sleep 1
done
grep -q "Your agent is connected" <<<"$status" || fail "Get started never saw the trace"
for _ in $(seq 10); do [[ "$(landing)" == "$UI/traces" ]] && break; sleep 1; done
[[ "$(landing)" == "$UI/traces" ]] || fail "after the first trace, / should go to /traces, got $(landing)"
pass "/ now goes to /traces"

code=$(curl -s -b "$jar" -c "$jar" -o /dev/null -w '%{http_code}' -X POST -H 'Sec-Fetch-Site: same-origin' "$UI/start/dismiss")
[[ "$code" == 303 ]] || fail "dismiss returned $code"
! page /traces | grep -q 'href="/start"' || fail "nav still offers Get started after dismiss"
pass "Get started can be dismissed"
