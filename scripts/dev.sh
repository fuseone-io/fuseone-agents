#!/usr/bin/env bash
# The whole platform on a laptop: database, API, worker, console, and local
# stand-ins for the two things an installation talks to but does not contain —
# a model provider and an MCP server.
#
# Nothing here is a special mode of the product. The worker runs the real
# client against a real chat-completions endpoint and the real MCP protocol; a
# local run exercises the same code an installation does.
set -euo pipefail

cd "$(dirname "$0")/.."

DSN=${TEST_DSN:-postgres://agents:agents@127.0.0.1:5433/agents}
# A fixed key so a restart can still read what the last run sealed. Fixed is
# right here and wrong everywhere else: this database is local and disposable,
# and a real installation generates its own with `agentd keygen`.
export FUSEONE_MASTER_KEY=${FUSEONE_MASTER_KEY:-ZGV2ZWxvcG1lbnQtb25seS1rZXktZG8tbm90LXVzZSE=}
# The same setup token every time, so claiming a fresh database is paste-free.
DEV_TOKEN=${DEV_TOKEN:-fuseone-development-setup-token}
API_ADDR=${API_ADDR:-127.0.0.1:8080}
MODEL_ADDR=${MODEL_ADDR:-127.0.0.1:8091}
SPECS=${SPECS:-dev/agents}

# Every child is recorded so `make stop` can end the stack from another
# terminal — or after the one that started it is gone, which is when an
# orphaned worker holding a lease is hardest to notice.
PIDFILE=${PIDFILE:-.dev/pids}
mkdir -p "$(dirname "$PIDFILE")"
: >"$PIDFILE"

pids=()
track() {
	pids+=("$1")
	echo "$1" >>"$PIDFILE"
}

cleanup() {
	trap - INT TERM EXIT
	[ ${#pids[@]} -eq 0 ] || kill "${pids[@]}" 2>/dev/null || true
	wait 2>/dev/null || true
	rm -f "$PIDFILE"
}
trap cleanup INT TERM EXIT

step() { printf '\n\033[1m%s\033[0m\n' "$*"; }

step "database"
docker compose up -d
until docker compose exec -T postgres pg_isready -U agents >/dev/null 2>&1; do sleep 1; done
go run ./cmd/agentd migrate --dsn "$DSN"

step "building"
go build -o bin/agentd ./cmd/agentd
go build -o bin/devstack ./cmd/devstack

step "model provider (stand-in)"
bin/devstack model --addr "$MODEL_ADDR" &
track $!
until curl -sf -o /dev/null "http://${MODEL_ADDR}/healthz"; do sleep 0.2; done

step "console"
# Started first: without the embedui tag the API proxies unmatched paths to the
# dev server, so waiting on the API before the console is a deadlock.
# Not silenced: when Vite picks a different port or fails to start, that
# message is the only thing that explains why the console is not there.
FUSEONE_WEB_DEV_URL="http://${API_ADDR}" npm --prefix web run dev -- --port 5173 --strictPort &
track $!

step "setup token"
# Fixed, so a reset does not mean hunting for a new string to paste. Safe here
# and nowhere else: this database is local and `agentd keygen`-style secrecy
# only matters for an installation somebody can reach. Minted before the server
# starts, so the server finds one and stays quiet — exactly one token in the
# log, and it is the one printed at the end.
if TOKEN=$(bin/agentd bootstrap --dsn "$DSN" --reissue --token "$DEV_TOKEN" 2>/dev/null); then
	SETUP="setup token   ${TOKEN}"
else
	SETUP="já reivindicada — recomece com: make reset"
fi
echo "${SETUP}"

step "api"
bin/agentd serve --dsn "$DSN" --addr "$API_ADDR" --base-url "http://${API_ADDR}" &
track $!
until curl -sf -o /dev/null "http://${API_ADDR}/api/v1/healthz"; do sleep 0.2; done

step "worker"
# The provider is configured the way an installation configures one: by
# environment. Pointing OPENAI_BASE_URL at the stand-in is the only difference.
OPENAI_API_KEY=dev \
OPENAI_BASE_URL="http://${MODEL_ADDR}" \
	bin/agentd worker --dsn "$DSN" --specs "$SPECS" \
		--mcp "crm=bin/devstack mcp" \
		--mcp "kb=bin/devstack mcp" &
track $!

# Publishing does not start an agent, and that is the product working: a
# specification arrives paused, because an agent nobody has decided about does
# not run (DE-07). In the console somebody presses Start. Here the stack does
# it so a fresh `make dev` has a run to look at rather than a first-run step
# that dies reporting the agent is paused — which is what it did until this
# existed, on the one command the README tells a newcomer to run first.
step "started"
docker compose exec -T postgres psql -U agents -d agents -qc \
	"update agent_state set paused = false, changed_by = 'make dev'" >/dev/null

step "first run"
sleep 1
bin/agentd start --dsn "$DSN" --agent suporte --by "$(whoami)"

cat <<EOF

  console   http://localhost:5173
  api       http://${API_ADDR}
  database  ${DSN}
  ${SETUP}

  open another run:
    bin/agentd start --dsn ${DSN} --agent suporte

Ctrl-C stops everything.
EOF

wait
