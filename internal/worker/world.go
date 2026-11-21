package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// CodeWorldUnreadable is the failure a replayed case ends with when its world cannot be
// honoured (NT-018 phase 5).
const CodeWorldUnreadable = "fuseone:world-unreadable"

// Worlds reads the world of a case at a version of its corpus, declared here by the consumer.
// A case with none answers nil.
type Worlds interface {
	WorldOf(ctx context.Context, agent domain.AgentID, version int64, caseID string) (*domain.CaseWorld, error)
}

// WithWorlds wires how the simulation pool reads the world a case is replayed with. The real
// pool never gets it: it runs no case.
func (w *Worker) WithWorlds(worlds Worlds) *Worker {
	w.worlds = worlds
	return w
}

/*
worldOf is the world a run replays: the one its case keeps at the version its opening names,
read only for a simulated run of a case of a version. A rehearsal waits for its author
instead, and a run of cases written for one simulation names no version to read from.

A pool that cannot read worlds refuses such a run rather than replay it without one: it cannot
know the case has none, and a case answered with nothing where its author answered something
would report held on a situation nobody corrected.
*/
func (w *Worker) worldOf(ctx context.Context, claim domain.Claim, started domain.RunStartedPayload) (*domain.CaseWorld, error) {
	if !started.Simulated || started.Rehearsal || started.Case == "" || started.DatasetVersion == 0 {
		return nil, nil
	}
	if w.worlds == nil {
		return nil, fmt.Errorf("%s: this pool does not read worlds: %w", claim.RunID, domain.ErrWorldUnreadable)
	}
	return w.worlds.WorldOf(ctx, claim.AgentID, started.DatasetVersion, started.Case)
}

// unreadable says whether a resolution failed for good: a seal or a world that cannot be
// honoured will not become readable by retrying.
func unreadable(reason error) (string, bool) {
	switch {
	case errors.Is(reason, domain.ErrCandidateUnreadable):
		return CodeCandidateUnreadable, true
	case errors.Is(reason, domain.ErrWorldUnreadable):
		return CodeWorldUnreadable, true
	}
	return "", false
}
