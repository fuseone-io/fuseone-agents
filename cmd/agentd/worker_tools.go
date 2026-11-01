package main

import (
	"context"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/worker"
)

/*
connectTools brings up the tool servers, from the flags and from the console.

The flags first: they are the way to point a laptop at a server before there is
an administrator to configure one, and the reconciler must not disconnect what
it did not connect.
*/
func (p *workerParts) connectTools(
	ctx context.Context, cfg workerFlags, metrics *worker.MetricsRegistry,
) error {
	cleanupStaleConfigFiles()
	egressObserver := newStdioEgressReporting(p.egress, metrics)
	egressObserver.start(ctx, stdioEgressFlush)

	reconcile := newReconciler(p.catalog, p.integrations, p.health).
		withCredentialPolicy(p.known).
		withProtocolPolicy(p.known).
		withStdioEgressObserver(egressObserver)
	if p.curator != nil {
		reconcile = reconcile.publishingTo(p.curator).classifyingWith(p.curator).
			holdingAs(cfg.owner, p.curator)
	}
	connectFlagServers(ctx, cfg.servers, reconcile)

	// Then the configured ones, and again on a timer. Before this, a server
	// added from the console offered nothing until somebody restarted the
	// worker, with nothing on any screen saying so.
	if p.integrations != nil {
		reconcile.reconcile(ctx)
	}
	// Published before the watch starts: the publication reads the
	// reconciler's state, which belongs to the watch once it runs.
	if p.curator != nil {
		if err := p.curator.Publish(ctx, reconcile.publication()); err != nil {
			return err
		}
	}
	if p.integrations != nil {
		p.startToolWatch(ctx, reconcile)
	}

	if p.curator == nil {
		return nil
	}
	if err := syncRulings(ctx, p.catalog, p.curator); err != nil {
		return err
	}
	go refreshRulings(ctx, p.catalog, p.curator)
	return nil
}

// startToolWatch hands the reconciler to its watch, which owns it from here
// on: every toolRefresh, or on each tick a journey sends.
func (p *workerParts) startToolWatch(ctx context.Context, reconcile *reconciler) {
	if p.toolTicks != nil {
		go reconcile.watch(ctx, p.toolTicks)
		return
	}
	go reconcile.watchEvery(ctx, toolRefresh)
}

// connectFlagServers hands the servers the --mcp flags name to the reconciler
// as ones it must never disconnect, and starts them. One that does not answer
// now is started again on a later pass, as one whose session dies is.
func connectFlagServers(ctx context.Context, servers []string, reconcile *reconciler) {
	for _, entry := range servers {
		name, command, _ := strings.Cut(entry, "=")
		// Accepted by having been typed. A server named on the command line
		// was put there by whoever starts this process, which is the same
		// decision the console asks for in a checkbox — asking again would be
		// asking the person who wrote the flag to confirm the flag.
		//
		// It carries no variables of its own: those live in the vault, and a
		// flag has no vault entry. A local server that needs a credential is
		// configured through the console.
		reconcile.hold(domain.MCPServer{
			Name: name, Transport: domain.TransportStdio, Command: command,
			AcceptsLocalExecution: true,
		})
	}
	reconcile.reviveFlagServers(ctx)
}
