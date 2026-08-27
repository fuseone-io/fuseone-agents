package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Two rows that are one identity are refused, not chosen between.

They exist because the platform allowed them: before the canonical key,
"Grafana Datasource" and "grafana  datasource" were two memories that never
found each other, and the index that finally reveals them is deliberately not
unique — a constraint would have refused the upgrade rather than surface the
duplicates already in the table.

What a write must not do is pick one. Merging into either records a correction
against half the fact and leaves the other half active, still answering the same
question differently, while the person who wrote the correction is told it
worked.

Planted rather than written through Assert, because Assert is precisely what can
no longer produce this shape. It is what an upgrade inherits.
*/
func TestAssert_twoRowsAreOneIdentity_refusesToChooseBetweenThem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemory()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	first := plantAssertion(t, store, "Grafana Datasource", now)
	second := plantAssertion(t, store, "grafana  datasource", now)
	if first.ID == second.ID {
		t.Fatal("the fixture planted one row; two spellings must be two rows")
	}

	_, err := store.Assert(ctx, correcting("Grafana Datasource"),
		"usr_ana", "correcting the claim", now.Add(time.Hour))
	if !errors.Is(err, ErrCanonicalConflict) {
		t.Fatalf("Assert = %v, want the write refused rather than one row chosen", err)
	}

	for _, planted := range []domain.MemoryAssertion{first, second} {
		held := store.values[planted.ID]
		if held.Claim != planted.Claim || !held.UpdatedAt.Equal(planted.UpdatedAt) {
			t.Errorf("row %s became %+v, want it untouched", planted.ID, held)
		}
	}
}

// The pair named is the same pair every time, so two people reading the same
// error are looking at the same two rows. Map order is not an ordering.
func TestAssert_twoRowsAreOneIdentity_namesTheSamePairEveryTime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	var seen string
	for i := 0; i < 20; i++ {
		store := NewMemory()
		plantAssertion(t, store, "Grafana Datasource", now)
		plantAssertion(t, store, "grafana  datasource", now)

		_, err := store.Assert(ctx, correcting("Grafana Datasource"),
			"usr_ana", "correcting the claim", now)
		if err == nil {
			t.Fatal("Assert succeeded on two rows of one identity")
		}
		if seen == "" {
			seen = err.Error()
			continue
		}
		if err.Error() != seen {
			t.Fatalf("conflict reported %q and %q, want one stable answer", seen, err.Error())
		}
	}
}

func plantAssertion(
	t *testing.T, m *Memory, subject string, now time.Time,
) domain.MemoryAssertion {
	t.Helper()
	a := correcting(subject)
	a.Claim = "written before the canonical key existed"
	a.CreatedBy, a.UpdatedBy = "usr_ana", "usr_ana"
	a.CreatedAt, a.UpdatedAt = now, now
	a.ID = domain.MemoryAssertionID(a)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[a.ID] = a
	return a
}

func correcting(subject string) domain.MemoryAssertion {
	return domain.MemoryAssertion{
		Scope:   domain.Scope{Company: "acme", Area: "platform"},
		AgentID: "triage", Kind: "incident", Subject: subject,
		Signature: "grafana.datasource.down",
		Claim:     "datasource errors clear after refreshing the datasource token",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-evidence-1", Artifact: "final_answer", Digest: "sha256:abcd",
		}},
		Observations: 2, Confirmed: 1, Status: domain.MemoryActive,
	}
}
