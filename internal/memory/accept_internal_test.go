package memory

import (
	"context"
	"errors"
	"strings"
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

/*
A proposal carrying a credential is refused even when nobody rewrites it.

The queue is not scanned when a suggestion is recorded, so the accept is the
first and only place a key in one is seen — and it is seen on the assertion the
merge is about to write, not on the request that asked for it. A check at the
edge inspected an empty claim and let the stored one straight through.

Subject and signature too. Somebody correcting the claim to something harmless
does not make the rest of the memory harmless.
*/
func TestAccept_aProposalShapedLikeACredential_isRefusedAndStaysPending(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	key := "-----BEGIN RSA PRIVATE KEY-----"
	better := "refresh the token"

	for _, c := range []struct {
		name  string
		plant func(*domain.MemorySuggestion)
		in    func(*AcceptInput)
	}{
		{"a key in the claim, accepted as written",
			func(s *domain.MemorySuggestion) { s.Claim = "the key is " + key }, nil},
		{"a key in the signature, with the claim rewritten",
			func(s *domain.MemorySuggestion) { s.Signature = key },
			func(in *AcceptInput) { in.Claim = &better }},
		{"a key in the subject, with the claim rewritten",
			func(s *domain.MemorySuggestion) { s.Subject = key },
			func(in *AcceptInput) { in.Claim = &better }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := NewMemory()
			held := plantedSuggestion(now)
			c.plant(&held)
			m.mu.Lock()
			m.suggestions[held.ID] = held
			m.mu.Unlock()

			in := AcceptInput{
				ID: held.ID, Scope: held.Scope, By: "usr_ana",
				Reason: "agreed", Now: now,
			}
			if c.in != nil {
				c.in(&in)
			}
			if _, err := m.AcceptSuggestion(context.Background(), in); !errors.Is(err, ErrSecret) {
				t.Fatalf("AcceptSuggestion = %v, want the credential refused", err)
			}
			if _, wrote := m.values[held.AssertionID]; wrote {
				t.Error("an assertion was written from a proposal carrying a key")
			}
			if m.suggestions[held.ID].Status != domain.MemorySuggestionPending {
				t.Error("the proposal was spent by a refusal")
			}
		})
	}
}

/*
An override at the accept marks the memory, exactly as it does at creation.

The flag is a request field and disappears with the request. The label is what
puts the decision in the row, in the list and in the event detail — and an
override nobody can see later is a guard that quietly stopped applying.
*/
func TestAccept_overridingASuspicion_marksTheMemory(t *testing.T) {
	t.Parallel()
	m := NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	held := plantedSuggestion(now)
	held.Claim = "the correlation id was aB3" + strings.Repeat("xY7z", 10)
	m.mu.Lock()
	m.suggestions[held.ID] = held
	m.mu.Unlock()

	in := AcceptInput{
		ID: held.ID, Scope: held.Scope, By: "usr_ana", Reason: "agreed", Now: now,
	}
	if _, err := m.AcceptSuggestion(context.Background(), in); !errors.Is(err, ErrSecretSuspected) {
		t.Fatalf("AcceptSuggestion = %v, want the question asked first", err)
	}

	in.Override = true
	accepted, err := m.AcceptSuggestion(context.Background(), in)
	if err != nil {
		t.Fatalf("AcceptSuggestion with the override: %v", err)
	}
	if !accepted.Labels.Has(domain.LabelSecret) {
		t.Errorf("labels = %v, want the row to carry that the question was raised", accepted.Labels)
	}
}

func plantedSuggestion(now time.Time) domain.MemorySuggestion {
	s := domain.MemorySuggestion{
		Scope: domain.Scope{Company: "acme", Area: "platform"}, AgentID: "triage",
		Kind: "incident", Subject: "grafana datasource",
		Signature: "grafana.datasource.down",
		Claim:     "datasource errors clear after refreshing the token",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-1", Artifact: "final_answer", Digest: "sha256:abcd",
		}},
		Observations: 1, Status: domain.MemorySuggestionPending,
		CreatedBy: "agent:triage", CreatedAt: now, UpdatedBy: "agent:triage", UpdatedAt: now,
	}
	s.AssertionID = domain.MemoryAssertionID(domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID, Kind: s.Kind,
		Subject: s.Subject, Signature: s.Signature,
	})
	s.ID = domain.MemorySuggestionID(s)
	return s
}
