package main

import (
	"context"
	"log/slog"
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
	ctx context.Context, servers []string, metrics *worker.MetricsRegistry,
) error {
	cleanupStaleConfigFiles()
	egressObserver := newStdioEgressReporting(p.egress, metrics)
	egressObserver.start(ctx, stdioEgressFlush)

	reconcile := newReconciler(p.catalog, p.integrations, p.health).
		withCredentialPolicy(p.known).
		withProtocolPolicy(p.known).
		withStdioEgressObserver(egressObserver)
	if p.curator != nil {
		reconcile = reconcile.publishingTo(p.curator).classifyingWith(p.curator)
	}

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
		if err := connectServer(ctx, p.catalog, domain.MCPServer{
			Name: name, Transport: domain.TransportStdio, Command: command,
			AcceptsLocalExecution: true,
		}, domain.MCPCredentials{}, nil, nil, credentialPolicy{}, egressObserver); err != nil {
			slog.Error("tool server did not answer; its tools are unavailable",
				"server", name, "err", err)
			observe(ctx, p.health, name, false, 0, err.Error())
			continue
		}
		reconcile.hold(name)
		count := p.catalog.CountFrom(name)
		slog.Info("tool server connected", "server", name, "transport", "stdio", "tools", count)
		observe(ctx, p.health, name, true, count, "")
	}

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
		go reconcile.watch(ctx, toolRefresh)
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
