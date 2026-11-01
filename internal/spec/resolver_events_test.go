package spec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
)

/*
Whether the definition declares events reaches the run's Start from the
registry the worker resolves through (#203). Dropped on the way, a run at
the end of an event chain finishes with no record that its events were
withheld, and the listener that never started has no reason on the trail.
*/
func TestResolve_declaredEvents_reachTheRunsStart(t *testing.T) {
	cases := map[string]struct {
		definition string
		want       bool
	}{
		"emits declared": {strings.Replace(definition, "tools:", "emits:\n  - ticket.triado\ntools:", 1), true},
		"emits absent":   {definition, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			registry := openRegistry(t)
			ctx := context.Background()
			version := published(t, c.definition)
			if err := registry.Publish(ctx, version, "usr_ana", "acme"); err != nil {
				t.Fatalf("Publish: %v", err)
			}
			providers := model.NewRegistry(nil)
			if err := providers.Register(model.Provider{
				Name: "openai", Kind: model.KindOpenAICompatible,
				BaseURL: "http://127.0.0.1:1", APIKey: "test",
			}); err != nil {
				t.Fatalf("Register: %v", err)
			}

			resolved, err := spec.NewResolver(registry, providers, nil).Resolve(ctx, version.ID, version.Version)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if resolved.Start.PublishesEvents != c.want {
				t.Errorf("Start.PublishesEvents = %v, want %v", resolved.Start.PublishesEvents, c.want)
			}
		})
	}
}
