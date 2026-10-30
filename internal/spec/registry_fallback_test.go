package spec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

var declaredFallback = domain.ModelPair{Provider: "anthropic", Model: "claude-haiku-5"}

func withRegistryFallback(t *testing.T) string {
	t.Helper()
	return strings.Replace(definition, "tools:",
		"fallback:\n  provider: anthropic\n  model: claude-haiku-5\ntools:", 1)
}

func TestPublish_theFallbackPair_survivesTheRegistry(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	declaring := published(t, withRegistryFallback(t))

	if err := r.Publish(ctx, declaring, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// The worker resolves from here. A column the read forgets is a fallback
	// the author reviewed and no run ever reaches.
	got, err := r.Get(ctx, "triage", declaring.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Fallback != declaredFallback {
		t.Errorf("Fallback = %+v, want %+v", got.Fallback, declaredFallback)
	}
}

func TestList_theFallbackPair_isHandedBackToTheEditor(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	if err := r.Publish(ctx, published(t, withRegistryFallback(t)), "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// What a read omits an editor cannot put back, and publishing again would
	// delete it.
	listed, err := r.List(ctx, domain.Scope{}, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Fallback != declaredFallback {
		t.Fatalf("listed = %+v, want the fallback pair", listed)
	}
	versions, err := r.Versions(ctx, "triage")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 1 || versions[0].Fallback != declaredFallback {
		t.Fatalf("versions = %+v, want the fallback pair", versions)
	}
}

func TestGet_aVersionPublishedWithoutAFallback_declaresNone(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	plain := published(t, definition)
	if err := r.Publish(ctx, plain, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got, err := r.Get(ctx, "triage", plain.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Fallback.IsZero() {
		t.Errorf("Fallback = %+v, want none", got.Fallback)
	}
}
