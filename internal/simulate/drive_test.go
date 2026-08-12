package simulate_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/simulate"
)

type fakeAdvancer struct {
	phases []engine.Phase
	turns  int
}

func (f *fakeAdvancer) Advance(context.Context, engine.Start) (engine.Status, error) {
	phase := engine.PhaseRunning
	if f.turns < len(f.phases) {
		phase = f.phases[f.turns]
	}
	f.turns++
	return engine.Status{Phase: phase}, nil
}

func TestDrive_stopsWhenTheRunFinishes(t *testing.T) {
	t.Parallel()

	adv := &fakeAdvancer{phases: []engine.Phase{engine.PhaseRunning, engine.PhaseFinished}}
	status, err := simulate.Drive(t.Context(), adv, engine.Start{}, 10)
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	if status.Phase != engine.PhaseFinished || adv.turns != 2 {
		t.Errorf("phase %v after %d turns", status.Phase, adv.turns)
	}
}

func TestDrive_stopsWhenAPersonWouldBeAsked(t *testing.T) {
	t.Parallel()

	adv := &fakeAdvancer{phases: []engine.Phase{engine.PhaseAwaitingApproval}}
	status, _ := simulate.Drive(t.Context(), adv, engine.Start{}, 10)

	// Waiting for a person is a result, not a pause to wait out. "It would
	// have asked you here" is the sentence a simulation exists to produce,
	// and answering the approval on the author's behalf would invent the one
	// decision the product refuses to make for anybody.
	if status.Phase != engine.PhaseAwaitingApproval || adv.turns != 1 {
		t.Errorf("phase %v after %d turns", status.Phase, adv.turns)
	}
}

func TestDrive_boundsAPlannerThatNeverFinishes(t *testing.T) {
	t.Parallel()

	adv := &fakeAdvancer{}
	if _, err := simulate.Drive(t.Context(), adv, engine.Start{}, 3); err == nil {
		t.Fatal("want a refusal")
	}
	// A run bounded only by its budget would spend the whole ceiling of every
	// case in a set of fifty before anybody saw a report.
	if adv.turns != 3 {
		t.Errorf("took %d turns", adv.turns)
	}
}
