package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/connectortools"
)

const connectorRefresh = 30 * time.Second

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
