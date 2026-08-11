#!/usr/bin/env bash
# Smoke test for the shipping artefact.
#
# Everything here is a property of the built binary rather than of the code:
# what the embedded console serves, what it refuses to serve, and whether an
# unauthenticated caller is turned away. A unit suite cannot see any of it.
set -euo pipefail

BIN=${BIN:-bin/agentd}
DSN=${TEST_DSN:-postgres://agents:agents@127.0.0.1:5433/agents}
ADDR=${SMOKE_ADDR:-127.0.0.1:8099}
BASE="http://${ADDR}"

"$BIN" serve --demo --dsn "$DSN" --addr "$ADDR" >/tmp/agentd-smoke.log 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
	curl -sf -o /dev/null "${BASE}/api/v1/healthz" && break
	sleep 0.2
done
# A server that never came up would otherwise fail every check below with a
# connection error, which reads as seven broken behaviours rather than one
# process that did not start.
if ! curl -sf -o /dev/null "${BASE}/api/v1/healthz"; then
	echo "the server did not become ready; last lines of its log:"
	tail -5 /tmp/agentd-smoke.log
	exit 1
fi

failures=0
check() { # check <description> <expected status> <path>
	actual=$(curl -s -o /dev/null -w '%{http_code}' "${BASE}$3")
	if [ "$actual" != "$2" ]; then
		printf '  FAIL  %-52s want %s, got %s\n' "$1" "$2" "$actual"
		failures=$((failures + 1))
	else
		printf '  ok    %-52s %s\n' "$1" "$actual"
	fi
}

JS=$(curl -s "${BASE}/" | grep -o '/assets/index-[A-Za-z0-9_-]*\.js' | head -1)
[ -n "$JS" ] || { echo "the console's bundle is not referenced from index.html"; exit 1; }

check "the console loads"                      200 "/"
check "a deep link reaches the router"         200 "/runs/run_a4d76"
check "the hashed bundle is served"            200 "$JS"
check "no sourcemap ships with the binary"     404 "${JS}.map"
check "a missing asset is not the app shell"   404 "/assets/nope.js"
check "an unauthenticated caller is refused"   401 "/api/v1/me"
check "liveness answers without a credential"  200 "/api/v1/healthz"
# Authentication wraps the whole /api/ prefix, so an unknown path answers 401
# rather than 404: an anonymous caller learns nothing about which endpoints
# exist. What must never happen is the console's HTML being returned to
# something that asked for the API.
api_type=$(curl -s -o /dev/null -w '%{content_type}' "${BASE}/api/v1/nope")
case "$api_type" in
*html*)
	printf '  FAIL  %-52s got %s\n' "an API path never answers with the console" "$api_type"
	failures=$((failures + 1))
	;;
*)
	printf '  ok    %-52s %s\n' "an API path never answers with the console" "$api_type"
	;;
esac

if curl -s "${BASE}${JS}" | grep -q sourceMappingURL; then
	echo "  FAIL  the bundle advertises a sourcemap"
	failures=$((failures + 1))
fi

[ "$failures" -eq 0 ] || { echo "smoke: ${failures} failed"; exit 1; }
echo "smoke: the artefact serves what it should"
