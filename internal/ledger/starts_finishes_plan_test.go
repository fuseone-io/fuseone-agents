package ledger_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The dispatcher's batch read stays one statement over the batch's months
(#225), now that it carries each run's claim (#204, slice 3).
*/
func TestStartsAndFinishes_includesClaim_stillOneStatementPrunedToTheBatchsMonths(t *testing.T) {
	s, pool, counter := countedLedger(t)
	first, second := trailMonths(t, pool)
	var batch []domain.RunSummary
	for i, at := range []time.Time{first, second, second.Add(time.Hour)} {
		id := domain.RunID(fmt.Sprintf("run-%d", i))
		started := step(id, domain.StepRunStarted)
		started.AgentID, started.At = "developer", at
		mustAppend(t, s, started)
		claimed := claimStep(t, id, "developer", domain.WorkItemRef(fmt.Sprintf("%s%d", card, i)))
		claimed.At = at
		mustAppend(t, s, claimed)
		finished := step(id, domain.StepRunFinished)
		finished.AgentID = "developer"
		mustAppend(t, s, finished)
		batch = append(batch, domain.RunSummary{RunID: id, StartedAt: at})
	}

	counter.take()
	got, err := s.StartsAndFinishes(context.Background(), batch)
	if err != nil {
		t.Fatalf("StartsAndFinishes: %v", err)
	}
	if asked := counter.take(); len(asked) != 1 {
		t.Errorf("the batch ran %d statements, want 1: %q", len(asked), asked)
	}
	for _, r := range batch {
		if steps := got[r.RunID]; len(steps) != 3 || steps[1].Kind != domain.StepWorkItemClaimed {
			t.Errorf("%s: %+v, want its start, claim and finish", r.RunID, steps)
		}
	}

	ids := []string{"run-0", "run-1", "run-2"}
	moments := []time.Time{first, second, second.Add(time.Hour)}
	plan := explainJSON(t, pool, ledger.StartsAndFinishesSQL, ids, moments)
	var read []string
	for _, rel := range scannedRelations(plan) {
		if strings.HasPrefix(rel, "run_steps") {
			read = append(read, rel)
		}
	}
	want := []string{partitionOf(first), partitionOf(second)}
	t.Logf("the batch reads %v", read)
	if !slices.Equal(read, want) {
		t.Errorf("the batch reads %v; want only its months %v", read, want)
	}
}
