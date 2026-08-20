package spec_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/spec"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time {
	return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
}

func TestPublisher_publishMakesTheNewVersionCurrentWithoutPausingTheAgent(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	publisher := spec.NewPublisher(pool, fixedClock{})
	registry := spec.NewRegistry(pool)

	first := published(t, definition)
	if err := publisher.Publish(ctx, first, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish(first): %v", err)
	}
	if err := publisher.SetPaused(ctx, first.ID, false, "usr_ops"); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}

	second := published(t, strings.Replace(
		definition,
		"Read the ticket and classify it.",
		"Read the ticket, consult the runbook, and classify it.",
		1,
	))
	if second.Version == first.Version {
		t.Fatal("test setup did not change the version")
	}
	if err := publisher.Publish(ctx, second, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish(second): %v", err)
	}

	versions, err := registry.Versions(ctx, first.ID)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) == 0 || versions[0].VersionID != second.Version {
		t.Fatalf("current version = %+v, want the just-published version %s",
			versions, second.Version)
	}
	paused, err := publisher.IsPaused(ctx, first.ID)
	if err != nil {
		t.Fatalf("IsPaused: %v", err)
	}
	if paused {
		t.Fatal("publishing a new version paused an agent somebody had started")
	}
}
