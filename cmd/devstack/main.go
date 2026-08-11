// Command devstack stands in for the two things an installation talks to but
// does not contain: a model provider and an MCP server.
//
// It exists so the whole platform can be exercised on a laptop with no API
// key, no network and no external system — and so that what runs locally is
// the real client code, over real HTTP and the real MCP protocol, rather than
// a mode the product only has in development.
//
// It is not part of the shipping artefact: `make build` builds ./cmd/agentd.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, `devstack — local stand-ins for a model provider and an MCP server

usage:
  devstack model [--addr host:port]  serve a chat-completions endpoint
  devstack mcp                       serve tools over stdio
`)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "model":
		err = serveModel(os.Args[2:])
	case "mcp":
		err = serveMCP(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "devstack:", err)
		os.Exit(1)
	}
}
