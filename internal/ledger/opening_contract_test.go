package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A run's opening, read alone.

Every turn of every pool reads it before resolving anything (NT-016), so it must cost one
step and not the run: a long run read whole on every turn is a scan that grows with the
run, on top of the runner's own reads.
*/
func TestOpening_ofALongRun_isItsFirstStepAlone(t *testing.T) {
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	run(t, "a long run's opening", func(t *testing.T, s Store) {
		ctx := context.Background()
		opening := startedAt("run-long", base)
		opening.Payload = mustJSON(t, domain.RunStartedPayload{Trigger: "manual", Case: "case-1"})
		first, err := s.Append(ctx, opening)
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		for i := 0; i < 40; i++ {
			planned := step("run-long", domain.StepPlanned)
			planned.At = base.Add(time.Duration(i+1) * time.Second)
			mustAppend(t, s, planned)
		}

		got, err := s.Opening(ctx, "run-long")
		if err != nil {
			t.Fatalf("Opening: %v", err)
		}
		if got.Seq != domain.FirstSeq || got.Kind != domain.StepRunStarted ||
			string(got.Payload) != string(first.Payload) || string(got.Hash) != string(first.Hash) {
			t.Errorf("opening = seq %d %s %s, want the sealed first step", got.Seq, got.Kind, got.Payload)
		}
	})
}

func TestOpening_ofARunThatDoesNotExist_isNotFound(t *testing.T) {
	run(t, "no such run", func(t *testing.T, s Store) {
		if _, err := s.Opening(context.Background(), "run-never"); !errors.Is(err, ledger.ErrNotFound) {
			t.Errorf("Opening = %v, want ErrNotFound", err)
		}
	})
}
