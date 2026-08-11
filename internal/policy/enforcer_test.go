package policy_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/policy"
)

// The Gate is on the path of every effect. Whether it enforces must not depend
// on a query succeeding.

type source struct {
	set policy.Set
	err error
}

func (s *source) Active(context.Context) (policy.Set, error) {
	if s.err != nil {
		return policy.Set{}, s.err
	}
	return s.set, nil
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func write() gate.Request {
	return gate.Request{
		Scope: domain.Scope{Company: "acme", Area: "cx"}, RunID: "run-1", AgentID: "triage",
		Seq: 2, Tool: "crm.reply", Effect: domain.EffectWrite,
		Pack:   gate.NewPack("crm.reply"),
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 20, Steps: 50},
	}
}

func denyAll() policy.Set {
	return policy.Set{Hash: "pol_deny", Policies: []domain.Policy{{
		Code: "POL-114", Resource: "*", Reach: domain.ReachInstallation,
		Effect: domain.PolicyDeny, Mode: domain.PolicyEnforce, Enabled: true,
	}}}
}

func TestEnforcer_beforeTheFirstRefresh_decidesUnderTheSafeDefault(t *testing.T) {
	t.Parallel()

	// Never under nothing. A worker that started a second before the database
	// answered must not allow a write it would otherwise escalate.
	e := policy.NewEnforcer(&source{set: denyAll()}, quiet())

	got, err := e.Evaluate(context.Background(), write())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got.Verdict != domain.VerdictRequireApproval {
		t.Errorf("verdict = %v, want the built-in ladder", got.Verdict)
	}
}

func TestEnforcer_afterRefresh_decidesUnderTheAuthoredSet(t *testing.T) {
	t.Parallel()

	e := policy.NewEnforcer(&source{set: denyAll()}, quiet())
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	got, _ := e.Evaluate(context.Background(), write())
	if got.Verdict != domain.VerdictBlock {
		t.Errorf("verdict = %v, want the authored deny", got.Verdict)
	}
	if got.PolicyHash != "pol_deny" {
		t.Errorf("hash = %q, want the snapshot that decided", got.PolicyHash)
	}
}

func TestEnforcer_refreshThatFails_leavesTheLastSetDeciding(t *testing.T) {
	t.Parallel()

	// The behaviour worth having: a worker that stops enforcing because a
	// query timed out is worse than one deciding under a set a minute old.
	src := &source{set: denyAll()}
	e := policy.NewEnforcer(src, quiet())
	if err := e.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	src.err = errors.New("database is having a moment")
	if err := e.Refresh(context.Background()); err == nil {
		t.Fatal("a failed refresh was reported as success")
	}

	got, _ := e.Evaluate(context.Background(), write())
	if got.Verdict != domain.VerdictBlock {
		t.Errorf("verdict = %v, want the last set still enforcing", got.Verdict)
	}
}

func TestEnforcer_unchangedSet_isNotSwappedIn(t *testing.T) {
	t.Parallel()

	// Swapping on every tick would rebuild the Gate sixty times an hour for
	// nothing, and log a policy change that did not happen.
	e := policy.NewEnforcer(&source{set: denyAll()}, quiet())
	ctx := context.Background()

	if err := e.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	first := e.Hash()
	if err := e.Refresh(ctx); err != nil {
		t.Fatalf("Refresh again: %v", err)
	}
	if e.Hash() != first {
		t.Errorf("hash moved from %s to %s with no change", first, e.Hash())
	}
}
