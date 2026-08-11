#!/usr/bin/env bash
# Ends everything `make dev` started, from any terminal.
#
# Ctrl-C in the window that started the stack does the same thing. This exists
# for when that window is gone: a worker outlives its terminal happily, and an
# orphan holding a lease is the kind of thing you discover much later.
set -uo pipefail

cd "$(dirname "$0")/.."

PIDFILE=${PIDFILE:-.dev/pids}
stopped=0

if [ -f "$PIDFILE" ]; then
	while read -r pid; do
		[ -n "$pid" ] || continue
		if kill "$pid" 2>/dev/null; then
			stopped=$((stopped + 1))
		fi
	done <"$PIDFILE"
	rm -f "$PIDFILE"
fi

# A pidfile only covers a stack that shut down cleanly enough to write one.
# Anything left over is still ours to end.
for pattern in 'bin/agentd (serve|worker)' 'bin/devstack' 'vite --port 5173'; do
	while read -r pid; do
		[ -n "$pid" ] || continue
		kill "$pid" 2>/dev/null && stopped=$((stopped + 1))
	done < <(pgrep -f "$pattern" 2>/dev/null)
done

# The container goes, its data stays. Stopping the stack should not cost you
# the installation you claimed; `make reset` is how you ask for that.
docker compose down --remove-orphans >/dev/null 2>&1 || true

echo "stopped ${stopped} process(es); database kept (make reset to clear it)"
