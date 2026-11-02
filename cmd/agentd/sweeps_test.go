package main

import (
	"testing"
	"time"
)

// The backlog is read on the first sweep — a worker that just started says
// where dispatch stands at once — and then once a minute, not on every
// fifteen-second sweep: every replica reads it, during the incidents it
// describes (#228).
func TestBacklogDue_readsOnTheFirstSweepAndThenOnceAMinute(t *testing.T) {
	t.Parallel()
	var due []int
	for sweep := 0; sweep < 9; sweep++ {
		if backlogDue(sweep) {
			due = append(due, sweep)
		}
	}
	if len(due) != 3 || due[0] != 0 || due[1] != 4 || due[2] != 8 {
		t.Fatalf("read on sweeps %v; want 0, 4 and 8", due)
	}
	if every := eventSweep * time.Duration(due[1]-due[0]); every != time.Minute {
		t.Fatalf("read every %v; want once a minute", every)
	}
}
