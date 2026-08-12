package simulate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

func serviceFor(t *testing.T, queue int) (*simulate.Service, *ledger.Memory) {
	t.Helper()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	exec := simulate.NewExecutor(openerFor(store, content), depsFor(store, content, &liveTools{}))
	svc := simulate.NewService(exec, store, queue)

	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _ = svc.Run(ctx) }()
	t.Cleanup(func() {
		stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// A loop that outlives its context is a goroutine with no owner.
			t.Error("the service did not stop when its context did")
		}
	})
	return svc, store
}

// settles waits for a simulation to reach the number of cases it was given.
func settles(t *testing.T, svc *simulate.Service, id string, want int) simulate.Report {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		report, err := svc.Report(t.Context(), id)
		if err != nil {
			t.Fatalf("Report: %v", err)
		}
		if len(report.Cases) == want && !report.Running {
			return report
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("simulation %s never settled", id)
	return simulate.Report{}
}

func TestService_runsASubmittedJobAndReportsItFromTheLedger(t *testing.T) {
	t.Parallel()

	svc, _ := serviceFor(t, 2)
	job := jobFor("crm.lookup", `{"n":1}`, `{"n":2}`)
	if err := svc.Submit(job); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got := settles(t, svc, job.ID, 2)
	// Read back from the ledger rather than from whatever the executor was
	// holding: the report has to survive the process that produced it.
	if got.Agent != "suporte" || got.Expected != 2 {
		t.Errorf("report = %+v", got)
	}
	for i, c := range got.Cases {
		if c.Settled != simulate.SettledFinished {
			t.Errorf("case %d settled %q", i+1, c.Settled)
		}
	}
}

func TestService_whenTheQueueIsFull_refusesRatherThanSpawningWork(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	exec := simulate.NewExecutor(openerFor(store, content), depsFor(store, content, &liveTools{}))
	// No Run loop: nothing drains the queue, so the second submission finds
	// it full. Simulations cost real money at a real provider, and an
	// unbounded queue behind an HTTP handler is how an afternoon of them gets
	// started by accident.
	svc := simulate.NewService(exec, store, 1)

	if err := svc.Submit(jobFor("crm.lookup", `{"n":1}`)); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second := jobFor("crm.lookup", `{"n":2}`)
	second.ID = "sim-2"
	if err := svc.Submit(second); !errors.Is(err, simulate.ErrBusy) {
		t.Fatalf("second submit = %v, want ErrBusy", err)
	}
}

func TestService_aSimulationNobodyRan_isEmptyRatherThanAnError(t *testing.T) {
	t.Parallel()

	svc, _ := serviceFor(t, 1)
	got, err := svc.Report(t.Context(), "sim-never")
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(got.Cases) != 0 || got.Running {
		t.Errorf("report = %+v", got)
	}
}

func TestService_whileItRuns_theReportSaysSo(t *testing.T) {
	t.Parallel()

	svc, _ := serviceFor(t, 2)
	job := jobFor("crm.lookup", `{"n":1}`, `{"n":2}`, `{"n":3}`)
	if err := svc.Submit(job); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Expected is what tells a reader that three of three is the whole set
	// and two of three is a page still being written.
	got := settles(t, svc, job.ID, 3)
	if got.Expected != 3 || got.Running {
		t.Errorf("report = %+v", got)
	}
}
