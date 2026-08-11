# Local development stack

Fixtures for running the whole platform on a laptop. Nothing here ships.

- `agents/suporte.agent.md` — an agent whose tools are the ones `devstack mcp`
  serves, and whose provider is the local `devstack model`. The definition in
  `../agents/` is the product's example and points at a real provider.

Start everything with `make dev`.
