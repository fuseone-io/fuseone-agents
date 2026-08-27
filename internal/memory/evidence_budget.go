package memory

import (
	"fmt"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// Which citations a memory keeps when it cannot keep them all.
//
// The eight-record cap is the whole reason this is not a union. What decides the
// order is provenance: a citation carrying a label the assertion also carries is
// what makes that label auditable, and dropping it leaves a memory that says
// untrusted and points at nothing that is.

/*
mergedEvidence folds both sides and keeps what the assertion needs to explain
itself.

Order of preference, and the reason for it: a citation carrying a label the
assertion also carries is what makes that label auditable, and dropping it
leaves a memory that says untrusted and points at nothing that is. Everything
else competes on being recent, because a fact is best explained by the last
time it was seen.

A label no citation carries is not expected to be explained — nothing derived it
from a run, so there is nothing to point at. Scope labels are usually not in
that group: evidence resolved from the ledger carries company and area along
with whatever taint the run had.
*/
func mergedEvidence(
	stored, incoming []domain.MemoryEvidence, labels domain.Labels,
) ([]domain.MemoryEvidence, error) {
	// Newest first, then folded: boundedEvidence keeps the first of each key,
	// so ordering before it is what makes "recent" the tiebreak.
	all := boundedEvidenceOf(append(slices.Clone(incoming), stored...))
	if len(all) <= domain.MaxMemoryEvidence {
		return all, nil
	}

	kept := make([]domain.MemoryEvidence, 0, domain.MaxMemoryEvidence)
	taken := map[string]bool{}
	for _, label := range provenanceLabels(all, labels) {
		// A citation resolved from a run carries the whole accumulated set —
		// company, area, whatever taint the run had — so one record explaining
		// several labels is the ordinary case. Looking for a distinct carrier
		// per label would spend the budget on records that add nothing.
		if slices.ContainsFunc(kept, func(ev domain.MemoryEvidence) bool {
			return ev.Labels.Has(label)
		}) {
			continue
		}
		i := slices.IndexFunc(all, func(ev domain.MemoryEvidence) bool {
			return !taken[ev.Key()] && ev.Labels.Has(label)
		})
		if i < 0 {
			continue
		}
		if len(kept) == domain.MaxMemoryEvidence {
			return nil, fmt.Errorf("%w: %s has no room to be shown", ErrEvidenceCannotExplain, label)
		}
		kept = append(kept, all[i])
		taken[all[i].Key()] = true
	}
	for _, ev := range all {
		if len(kept) == domain.MaxMemoryEvidence {
			break
		}
		if !taken[ev.Key()] {
			kept = append(kept, ev)
			taken[ev.Key()] = true
		}
	}
	return kept, nil
}

// provenanceLabels are the assertion's labels that some citation carries. A
// label no citation has was not derived from a run, so nothing is expected to
// explain it.
func provenanceLabels(evidence []domain.MemoryEvidence, labels domain.Labels) []string {
	var out []string
	for _, label := range labels {
		if slices.ContainsFunc(evidence, func(ev domain.MemoryEvidence) bool {
			return ev.Labels.Has(label)
		}) {
			out = append(out, label)
		}
	}
	return out
}

// boundedEvidenceOf folds by key without applying the cap, so the eviction rule
// above can decide what the cap costs.
func boundedEvidenceOf(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	out := make([]domain.MemoryEvidence, 0, len(in))
	at := map[string]int{}
	for _, ev := range in {
		if i, seen := at[ev.Key()]; seen {
			out[i].Labels = out[i].Labels.Union(ev.Labels)
			continue
		}
		at[ev.Key()] = len(out)
		out = append(out, ev)
	}
	return out
}
