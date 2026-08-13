// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

import (
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/worker"
)

// The two halves of the queue.
//
// The worker command is a separate process from the API on purpose: the two
// scale on different axes, and a pool that needs isolation — its own network
// policy, its own resource limits — becomes its own deployment without
// touching the server.

// simulations returns the half of the queue holding simulated runs, or nil
// when the store cannot serve one.
func simulations(store Store) worker.Queue {
	type simulatable interface{ Simulations() ledger.SimulationQueue }
	if s, ok := store.(simulatable); ok {
		return s.Simulations()
	}
	return nil
}

// simulationSlots keeps the simulation pool a fraction of the main one, and
// never zero: an installation running a single worker still has to be able to
// simulate, or an agent could never leave Draft.
func simulationSlots(concurrency int) int {
	if slots := concurrency / 2; slots > 0 {
		return slots
	}
	return 1
}
