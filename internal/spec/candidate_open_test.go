package spec_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// The opener resolves an experiment's seal through the reader: its version, its sealed
// scope, and the pause the agent actually has (NT-018 phase 2, decision 3).
func TestCandidateReader_resolvesASealForTheOpener(t *testing.T) {
	w := newCandidateWorld(t)
	ctx := context.Background()
	seal, version := w.seal(t, "finance", time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC))

	got, found, err := w.reader.Candidate(ctx, w.agent, seal)
	if err != nil || !found {
		t.Fatalf("Candidate = %v, %v", found, err)
	}
	if got.Agent != w.agent || got.Version != version || got.Scope != (domain.Scope{Company: "acme", Area: "finance"}) || got.Paused {
		t.Errorf("Candidate = %+v, want %s in acme/finance, not paused: an agent with no state has nothing to pause", got, version)
	}

	if _, err := w.pool.Exec(ctx, `insert into agent_state (agent_id, paused) values ($1, true)`, string(w.agent)); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if got, _, err := w.reader.Candidate(ctx, w.agent, seal); err != nil || !got.Paused {
		t.Errorf("with the agent paused: %+v (%v), want paused", got, err)
	}

	for name, probe := range map[string]struct {
		agent domain.AgentID
		seal  string
	}{
		"another agent's seal": {"someone-else", seal},
		"a seal nobody wrote":  {w.agent, "9f9f9f9f-8a43-4b55-9a3f-0d6a3c5f9e11"},
		"not a seal id":        {w.agent, "v1"},
	} {
		if _, found, err := w.reader.Candidate(ctx, probe.agent, probe.seal); err != nil || found {
			t.Errorf("%s: found=%v err=%v, want not found", name, found, err)
		}
	}
}

// The experiment route reads one seal's definition, checked as the worker checks it, before
// its first run: the sealed provider is what it asks about, and an unreadable seal refuses
// the first attempt.
func TestCandidateReader_readsOneSealsDefinition(t *testing.T) {
	w := newCandidateWorld(t)
	ctx := context.Background()
	seal, version := w.seal(t, "finance", time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC))

	got, err := w.reader.Sealed(ctx, w.agent, seal)
	if err != nil || got.Version != version || got.Provider != "openai" || got.Area != "finance" {
		t.Fatalf("Sealed = %s %s %s (%v), want the sealed definition", got.Version, got.Provider, got.Area, err)
	}
	if _, err := w.reader.Sealed(ctx, "someone-else", seal); !errors.Is(err, spec.ErrCandidateNotFound) {
		t.Errorf("another agent's seal: err = %v, want ErrCandidateNotFound", err)
	}
	w.erase(t, seal)
	if _, err := w.reader.Sealed(ctx, w.agent, seal); !errors.Is(err, domain.ErrCandidateUnreadable) {
		t.Errorf("an erased seal: err = %v, want ErrCandidateUnreadable", err)
	}
}
