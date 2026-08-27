package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/memory"
)

var mergeNow = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

func mergeable(claim string, labels ...string) domain.MemoryAssertion {
	return domain.MemoryAssertion{
		Scope: domain.Scope{Company: "acme", Area: "ops"}, AgentID: "triage",
		Kind: "incident", Subject: "slack alerts", Signature: "not_in_channel",
		Claim: claim, Status: domain.MemoryActive,
		Labels: domain.NewLabels(labels...),
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-1", Seq: 2, Artifact: "final_answer", Digest: "sha256:a",
			Labels: domain.NewLabels(labels...),
		}},
	}
}

func at(d time.Duration) *time.Time {
	t := mergeNow.Add(d)
	return &t
}

/*
The field rules, one table, because they are a decision and not a procedure.

Each row is a promise somebody could otherwise talk themselves out of: that a
correction cannot quietly drop a taint, that a count cannot go backwards, that
who created a memory survives whoever corrects it.
*/
func TestMerge_fieldRules(t *testing.T) {
	t.Parallel()

	stored := mergeable("the api returns not_in_channel", domain.LabelUntrusted)
	stored.Observations, stored.Confirmed = 4, 3
	stored.CreatedBy, stored.CreatedAt = "usr_ana", mergeNow.Add(-72*time.Hour)

	incoming := mergeable("the bot must be invited to the channel")
	incoming.Observations, incoming.Confirmed = 1, 1
	incoming.CreatedBy, incoming.CreatedAt = "usr_bruno", mergeNow
	incoming.UpdatedBy = "usr_bruno"

	got, outcome, err := memory.Merge(memory.MergeInput{
		Incoming: incoming, Stored: &stored, Origin: memory.OriginHuman, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if outcome != memory.Merged {
		t.Fatalf("outcome = %q, want merged", outcome)
	}

	for _, tc := range []struct {
		what string
		got  any
		want any
	}{
		{"the correction wins the claim", got.Claim, "the bot must be invited to the channel"},
		{"the taint survives a clean correction", got.Labels.Has(domain.LabelUntrusted), true},
		{"observations never go backwards", got.Observations, int64(4)},
		{"confirmations never go backwards", got.Confirmed, int64(3)},
		{"the creator is the first one", string(got.CreatedBy), "usr_ana"},
		{"the creation moment is the first one", got.CreatedAt, mergeNow.Add(-72 * time.Hour)},
		{"the last hand to touch it is recorded", string(got.UpdatedBy), "usr_bruno"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.what, tc.got, tc.want)
		}
	}
}

/*
Expiry, one table, because every row is a different question about the same
field and the wrong answer is invisible.

An assertion whose expiry has passed is projected as expired and stops being
readable, so a merge that failed to renew produces memory that exists, reports
active, and cannot be found.
*/
func TestMerge_expiryRules(t *testing.T) {
	t.Parallel()

	original := at(240 * time.Hour)
	renewed := mergeNow.Add(memory.DefaultMemoryTTL)

	for _, tc := range []struct {
		name        string
		origin      memory.MergeOrigin
		newEvidence bool
		want        time.Time
	}{
		{"correcting an active memory keeps its expiry", memory.OriginHuman, false, *original},
		{"re-asserting with new evidence renews it", memory.OriginHuman, true, renewed},
		{"accepting over an active memory keeps it", memory.OriginAccept, false, *original},
		{"accepting keeps it even when the suggestion cites something new",
			memory.OriginAccept, true, *original},
		{"auto-confirm after new observations renews it", memory.OriginAutoConfirm, true, renewed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stored := mergeable("stored")
			stored.ExpiresAt = original

			incoming := mergeable("incoming")
			if tc.newEvidence {
				incoming.Evidence = []domain.MemoryEvidence{{
					RunID: "run-2", Seq: 5, Artifact: "final_answer", Digest: "sha256:b",
				}}
			}

			got, _, err := memory.Merge(memory.MergeInput{
				Incoming: incoming, Stored: &stored, Origin: tc.origin, Now: mergeNow,
			})
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			if got.ExpiresAt == nil || !got.ExpiresAt.Equal(tc.want) {
				t.Errorf("expiry = %v, want %v", got.ExpiresAt, tc.want)
			}
		})
	}
}

// An accept must never shorten what is already there. A suggestion carries the
// learning policy's TTL, which is shorter than a memory a person has been
// correcting for months.
func TestMerge_acceptNeverShortensTheExpiry(t *testing.T) {
	t.Parallel()

	stored := mergeable("stored")
	stored.ExpiresAt = at(240 * time.Hour)
	incoming := mergeable("incoming")
	incoming.ExpiresAt = at(24 * time.Hour)

	got, _, err := memory.Merge(memory.MergeInput{
		Incoming: incoming, Stored: &stored, Origin: memory.OriginAccept, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !got.ExpiresAt.Equal(*stored.ExpiresAt) {
		t.Errorf("expiry = %v, want the longer one already stored", got.ExpiresAt)
	}
}

/*
A terminal state is a decision somebody made, and a write is not how it is
undone.

Without this an accept or an auto-confirm lands on a disabled memory, the
suggestion is consumed, and the assertion stays invisible: the queue empties and
nothing was learned.
*/
func TestMerge_terminalStates(t *testing.T) {
	t.Parallel()

	for _, status := range []domain.MemoryStatus{domain.MemoryDisabled, domain.MemorySourceErased} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			stored := mergeable("stored")
			stored.Status = status

			_, _, err := memory.Merge(memory.MergeInput{
				Incoming: mergeable("incoming"), Stored: &stored,
				Origin: memory.OriginHuman, Now: mergeNow,
			})
			if !errors.Is(err, memory.ErrMemoryTerminal) {
				t.Errorf("err = %v, want a terminal-state conflict", err)
			}
		})
	}
}

/*
Shared memory covers an agent's creation without becoming its target.

A run of one agent reads its own memory and the shared memory, so an equivalent
shared fact answers the need. Correcting it from there would let an action begun
in one agent's context rewrite what every agent reads, and nobody asked for that.
*/
func TestMerge_sharedMemoryCoveringAnAgentCreation_isNotTouched(t *testing.T) {
	t.Parallel()

	shared := mergeable("what everybody knows")
	shared.AgentID = ""
	before := shared.Claim

	got, outcome, err := memory.Merge(memory.MergeInput{
		Incoming: mergeable("what this agent would write"), Covering: &shared,
		Origin: memory.OriginHuman, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if outcome != memory.Covered {
		t.Fatalf("outcome = %q, want covered", outcome)
	}
	if got.Claim != before {
		t.Errorf("the shared claim became %q, want it untouched", got.Claim)
	}
}

// Nothing stored is an insert, and the incoming assertion is what it says it is.
func TestMerge_nothingStored_isAnInsert(t *testing.T) {
	t.Parallel()

	got, outcome, err := memory.Merge(memory.MergeInput{
		Incoming: mergeable("first"), Origin: memory.OriginHuman, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if outcome != memory.Inserted {
		t.Errorf("outcome = %q, want inserted", outcome)
	}
	if got.Claim != "first" {
		t.Errorf("claim = %q, want the one being written", got.Claim)
	}
}

/*
Evidence that explains a label is kept before evidence that is merely recent.

Keeping a taint while dropping the only citation that shows where it came from
leaves an assertion nobody can audit: it says untrusted and points at nothing
that is.
*/
func TestMerge_evictionKeepsWhatExplainsEachLabel(t *testing.T) {
	t.Parallel()

	stored := mergeable("stored")
	stored.Evidence = []domain.MemoryEvidence{{
		RunID: "run-tainted", Seq: 1, Artifact: "final_answer", Digest: "sha256:t",
		Labels: domain.NewLabels(domain.LabelUntrusted),
	}}
	stored.Labels = domain.NewLabels(domain.LabelUntrusted)

	// Enough clean citations to fill the budget on their own.
	incoming := mergeable("incoming")
	incoming.Evidence = nil
	for i := range domain.MaxMemoryEvidence {
		incoming.Evidence = append(incoming.Evidence, domain.MemoryEvidence{
			RunID: domain.RunID("run-clean"), Seq: int64(i + 10),
			Artifact: "final_answer", Digest: "sha256:c",
		})
	}

	got, _, err := memory.Merge(memory.MergeInput{
		Incoming: incoming, Stored: &stored, Origin: memory.OriginHuman, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	var explains bool
	for _, ev := range got.Evidence {
		if ev.Labels.Has(domain.LabelUntrusted) {
			explains = true
		}
	}
	if !explains {
		t.Errorf("evidence = %d records, none explaining the taint the assertion carries", len(got.Evidence))
	}
}

// When the budget cannot hold one citation per label, the merge refuses rather
// than storing a provenance it cannot show.
func TestMerge_capCannotExplainEveryLabel_refused(t *testing.T) {
	t.Parallel()

	stored := mergeable("stored")
	stored.Evidence = nil
	stored.Labels = nil
	for i := range domain.MaxMemoryEvidence + 1 {
		label := domain.LabelCompany(domain.CompanyID(string(rune('a' + i))))
		stored.Labels = stored.Labels.Union(domain.NewLabels(label))
		stored.Evidence = append(stored.Evidence, domain.MemoryEvidence{
			RunID: domain.RunID("run-" + string(rune('a'+i))), Seq: 1,
			Artifact: "final_answer", Digest: "sha256:x",
			Labels: domain.NewLabels(label),
		})
	}

	_, _, err := memory.Merge(memory.MergeInput{
		Incoming: mergeable("incoming"), Stored: &stored,
		Origin: memory.OriginHuman, Now: mergeNow,
	})
	if !errors.Is(err, memory.ErrEvidenceCannotExplain) {
		t.Errorf("err = %v, want a refusal to store a provenance it cannot show", err)
	}
}

// The identity is not something a merge decides. Whatever the incoming record
// says about who it is, the stored row keeps its name.
func TestMerge_identityIsNeverRewritten(t *testing.T) {
	t.Parallel()

	stored := mergeable("stored")
	stored.ID = "mem_original"

	incoming := mergeable("incoming")
	incoming.ID = "mem_someone_elses"
	incoming.Subject = "a different subject"

	got, _, err := memory.Merge(memory.MergeInput{
		Incoming: incoming, Stored: &stored, Origin: memory.OriginHuman, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got.ID != "mem_original" || got.Subject != "slack alerts" {
		t.Errorf("identity became %s/%q, want the stored one", got.ID, got.Subject)
	}
}

/*
One citation explains every label it carries.

Evidence resolved from a run arrives with the whole accumulated set on it —
company, area, and whatever taint the run had — so a single record explaining
several labels is the ordinary case. Looking for a different carrier per label
runs out of budget against memory that is perfectly explainable.
*/
func TestMerge_oneCitationExplainsSeveralLabels(t *testing.T) {
	t.Parallel()

	var labels domain.Labels
	for i := range domain.MaxMemoryEvidence + 1 {
		labels = labels.Union(domain.NewLabels(domain.LabelCompany(
			domain.CompanyID(string(rune('a' + i))))))
	}

	stored := mergeable("stored")
	stored.Labels = labels
	// One citation carrying every label, and one alternative carrier for each.
	// Hunting for a distinct record per label finds them, fills the budget, and
	// refuses memory the first citation already explained on its own.
	stored.Evidence = []domain.MemoryEvidence{{
		RunID: "run-rich", Seq: 1, Artifact: "final_answer", Digest: "sha256:a", Labels: labels,
	}}
	for i, label := range labels {
		stored.Evidence = append(stored.Evidence, domain.MemoryEvidence{
			RunID: "run-alt", Seq: int64(i + 2), Artifact: "final_answer",
			Digest: "sha256:b", Labels: domain.NewLabels(label),
		})
	}

	if _, _, err := memory.Merge(memory.MergeInput{
		Incoming: mergeable("incoming"), Stored: &stored,
		Origin: memory.OriginHuman, Now: mergeNow,
	}); err != nil {
		t.Errorf("Merge refused memory one citation explains entirely: %v", err)
	}
}

/*
A memory already at the record cap still renews when it learns a citation.

Counting records cannot see it: the ninth citation arrives, an older one is
evicted, and the count is eight either way. What changed is which citations
these are, so that is what the decision has to look at.
*/
func TestMerge_atTheEvidenceCap_newCitationStillRenews(t *testing.T) {
	t.Parallel()

	stored := mergeable("stored")
	stored.ExpiresAt = at(240 * time.Hour)
	stored.Evidence = nil
	for i := range domain.MaxMemoryEvidence {
		stored.Evidence = append(stored.Evidence, domain.MemoryEvidence{
			RunID: "run-old", Seq: int64(i + 1), Artifact: "final_answer", Digest: "sha256:o",
		})
	}

	incoming := mergeable("incoming")
	incoming.Evidence = []domain.MemoryEvidence{{
		RunID: "run-new", Seq: 99, Artifact: "final_answer", Digest: "sha256:n",
	}}

	got, _, err := memory.Merge(memory.MergeInput{
		Incoming: incoming, Stored: &stored, Origin: memory.OriginHuman, Now: mergeNow,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if want := mergeNow.Add(memory.DefaultMemoryTTL); !got.ExpiresAt.Equal(want) {
		t.Errorf("expiry = %v, want it renewed to %v", got.ExpiresAt, want)
	}
}

/*
An origin nobody set is not a human correction.

The field decides one thing — whether this write may renew the expiry — and the
zero value happens to mean the most permissive answer. Three call sites are
about to be wired to this, and forgetting the field on one of them would change
what a memory's life means without failing anything.
*/
func TestMerge_originThatIsNotOneOfThem_refused(t *testing.T) {
	t.Parallel()

	for _, origin := range []memory.MergeOrigin{"", "aceppt", "Human"} {
		t.Run(string(origin), func(t *testing.T) {
			t.Parallel()

			stored := mergeable("stored")
			_, _, err := memory.Merge(memory.MergeInput{
				Incoming: mergeable("incoming"), Stored: &stored, Now: mergeNow,
				Origin: origin,
			})
			if !errors.Is(err, memory.ErrMergeOrigin) {
				t.Errorf("err = %v, want a refusal to guess the origin", err)
			}
		})
	}
}
