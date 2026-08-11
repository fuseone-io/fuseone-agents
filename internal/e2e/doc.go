// Package e2e assembles the real platform and runs an agent through it.
//
// Every other suite in this repository tests one layer against doubles of its
// neighbours. This one wires the actual ledger, gate, worker, MCP catalogue,
// model client and specification store together, and substitutes only the two
// things that live outside the installation: the model provider's HTTP
// endpoint and the MCP server's implementation. Both are real protocol
// speakers, not interface stubs — the model is reached over HTTP by the real
// client, and the MCP server is the real SDK server over an in-memory
// transport.
//
// It exists because a platform whose layers each pass in isolation can still
// fail to run a single agent.
package e2e
