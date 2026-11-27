package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
unreached records a server the worker could not connect. One only a person can
discover, with nobody connected to discover as, waits for a person (#483,
delivery 3b): the observation says so, under the configuration it was made
with, so a reader honours it only while that is still the configuration.
Every other failure is recorded as what it is.
*/
func (r *reconciler) unreached(ctx context.Context, server domain.MCPServer, err error) {
	if r.health == nil {
		return
	}
	obs := domain.IntegrationHealth{
		Name: server.Name, Detail: err.Error(), ObservedAt: time.Now(), ObservedBy: hostname(),
	}
	if errors.Is(err, errNobodyToDiscoverAs) {
		obs.AwaitingPerson, obs.AwaitingGeneration = true, server.UpdatedAt
	}
	if err := r.health.Record(ctx, obs); err != nil {
		slog.Warn("could not record what an integration answered", "server", server.Name, "err", err)
	}
}
