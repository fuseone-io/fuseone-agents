package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/mcpmetrics"
)

// A run whose tools are on a server waiting for a person ends failed (#483,
// delivery 3b): no backing off brings a person, and a parked run would be
// resumed onto the same refusal. Connecting afterwards does not resume it.
func TestTurn_aServerWaitingForAPerson_endsTheRunFailed(t *testing.T) {
	t.Parallel()
	planner := &flakyPlanner{failures: 99, err: &engine.AwaitingPersonError{Server: "dpg"}}
	s := newSetup(t, Config{MaxAttempts: 5}, planner, nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if code := failedCode(t, s.store); code != mcpmetrics.CodePersonalCredentialMissing {
		t.Errorf("failed with %q, want %s", code, mcpmetrics.CodePersonalCredentialMissing)
	}
	if _, err := s.store.Claim(context.Background(), "other", time.Minute); !errors.Is(err, domain.ErrNoClaimableRun) {
		t.Errorf("Claim after the run failed = %v, want nothing left to retry", err)
	}
}

// callerWithoutCredential is a tool call finding the caller without their own
// credential: the same code, and not the refusal the run ends on.
type callerWithoutCredential struct{}

func (callerWithoutCredential) Error() string { return "dpg: no personal MCP credential" }

func (callerWithoutCredential) Summary() domain.FailureSummary {
	return domain.FailureSummary{Code: mcpmetrics.CodePersonalCredentialMissing}
}

// Recognised by its type, never by its code: a call that finds its caller
// without a credential parks, so somebody connects and resumes it.
func TestTurn_aCallWithoutTheCallersCredential_stillParks(t *testing.T) {
	t.Parallel()
	planner := &flakyPlanner{failures: 99, err: callerWithoutCredential{}}
	s := newSetup(t, Config{MaxAttempts: 5}, planner, nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if got := phaseOf(t, s.store); got != engine.PhaseParked {
		t.Fatalf("phase = %s, want parked", got)
	}
	if p := parkPayload(t, s.store); p.Reason != mcpmetrics.CodePersonalCredentialMissing {
		t.Errorf("parked = %+v, want the call's code", p)
	}
}
