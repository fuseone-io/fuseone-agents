# Local development stack

Fixtures for running the whole platform on a laptop. Nothing here ships.

- `agents/suporte.agent.md` — an agent whose tools are the ones `devstack mcp`
  serves, and whose provider is the local `devstack model`. The definition in
  `../agents/` is the product's example and points at a real provider.

It carries a fifteen-minute schedule so the scheduler is visible without
waiting for a working day — which also means a stack left running opens a run
every fifteen minutes, and each one stops for an approval. Drop the `triggers`
block if that gets in the way; the agent still runs from the console.

Start everything with `make dev`.
