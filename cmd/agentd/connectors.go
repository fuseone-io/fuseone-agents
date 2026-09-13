package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/connectortools"
)

const (
	connectorRefresh          = 30 * time.Second
	graviteeReconcileInterval = 15 * time.Second
)

func (p *workerParts) refreshConnectors(ctx context.Context) error {
	if p.native == nil || p.settings == nil {
		return nil
	}
	instances, err := connectortools.NewSettings(p.settings).Instances(ctx)
	if err != nil {
		return err
	}
	if err := p.native.SetInstances(instances); err != nil {
		return err
	}
	slog.Info("governed connector instances loaded", "count", len(instances))
	return nil
}

func (p *workerParts) watchConnectors(ctx context.Context) {
	ticker := time.NewTicker(connectorRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.refreshConnectors(ctx); err != nil {
				slog.Warn("governed connector refresh failed", "err", err)
			}
		}
	}
}

func (p *workerParts) reconcileGravitee(ctx context.Context, owner string) {
	if p.graviteeRuntime == nil || p.graviteeAttempts == nil {
		return
	}
	reconciler := connectortools.NewGraviteeAttemptReconciler(
		p.graviteeRuntime, p.graviteeAttempts, owner)
	ticker := time.NewTicker(graviteeReconcileInterval)
	defer ticker.Stop()
	for {
		count, err := reconciler.Sweep(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("Gravitee acceptance reconciliation failed", "err", err)
		}
		if count > 0 {
			slog.Info("Gravitee acceptance attempts reconciled", "attempts", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
