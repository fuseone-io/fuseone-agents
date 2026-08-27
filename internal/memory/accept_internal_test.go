package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A proposal the store would no longer accept does not become memory by being
agreed to.

assertionFromSuggestion never validated, so whatever the queue held reached the
merge as an assertion. A row recorded before a rule existed — or by a version
that had a different one — would be written as active memory on the strength of
somebody clicking accept, and the first thing to notice would be the next read.

Planted, because Suggest is exactly what can no longer produce it. It is the
shape a queue inherits.
*/
func TestAccept_aProposalTheStoreWouldNoLongerHold_isRefused(t *testing.T) {
	t.Parallel()
	m := NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	held := domain.MemorySuggestion{
		ID: "mems_legacy", AssertionID: "mem_legacy",
		Scope: domain.Scope{Company: "acme", Area: "platform"}, AgentID: "triage",
		Kind: "incident", Subject: "grafana datasource",
		// Empty, which the store has required for as long as it has validated.
		Signature: "",
		Claim:     "datasource errors clear after refreshing the token",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-1", Artifact: "final_answer", Digest: "sha256:abcd",
		}},
		Observations: 1, Status: domain.MemorySuggestionPending,
		CreatedBy: "agent:triage", CreatedAt: now, UpdatedBy: "agent:triage", UpdatedAt: now,
	}
	m.mu.Lock()
	m.suggestions[held.ID] = held
	m.mu.Unlock()

	_, err := m.AcceptSuggestion(context.Background(), AcceptInput{
		ID: held.ID, Scope: held.Scope, By: "usr_ana", Reason: "agreed", Now: now,
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("AcceptSuggestion = %v, want the proposal refused rather than promoted", err)
	}
	if _, wrote := m.values[held.AssertionID]; wrote {
		t.Error("an assertion was written from a proposal the store would not hold")
	}
}
