package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/memory"
)

type matchStore interface {
	mergeStore
	Disable(context.Context, string, domain.Scope, domain.UserID, string, time.Time) error
	Match(context.Context, memory.MatchInput) (memory.Match, error)
}

func matchFor(edit func(*memory.MatchInput)) memory.MatchInput {
	in := memory.MatchInput{
		Scope: platformScope, AgentID: "triage", Kind: "incident",
		Subject: "grafana datasource", Signature: "grafana.datasource.down",
		Now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC),
	}
	if edit != nil {
		edit(&in)
	}
	return in
}

/*
A memory spelled differently is still the memory somebody is about to duplicate.

The whole point of asking. If this answered only on the exact strings, it would
answer "nothing here" to the one person most likely to be told otherwise by the
merge a moment later — and the duplicate they were warned about would be the one
they just failed to be warned about.
*/
func TestMatch_findsTheMemoryWhateverItWasSpelledLike(t *testing.T) {
	t.Parallel()
	expectMatchFindsAnotherSpelling(t, context.Background(), memory.NewMemory())
}

func TestPostgresMatch_findsTheMemoryWhateverItWasSpelledLike(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMatchFindsAnotherSpelling(t, ctx, store)
}

func expectMatchFindsAnotherSpelling(t *testing.T, ctx context.Context, store matchStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	stored, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	got, err := store.Match(ctx, matchFor(func(in *memory.MatchInput) {
		in.Subject = "grafana  datasource"
	}))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Own == nil || got.Own.ID != stored.ID {
		t.Fatalf("own = %+v, want the memory already holding this identity", got.Own)
	}
	if !got.Answered() {
		t.Error("the match says there is nothing to show")
	}
}

/*
Every state, including the ones Find will not return.

Expired is the one that catches people out: the fact appears to be unknown, so
somebody teaches it again and cannot see why the duplicate exists. Disabled says
somebody decided against it, which is worth reading before deciding for it
again.
*/
func TestMatch_showsMemoryThatFindWillNotReturn(t *testing.T) {
	t.Parallel()
	expectMatchShowsTerminalStates(t, context.Background(), memory.NewMemory())
}

func TestPostgresMatch_showsMemoryThatFindWillNotReturn(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMatchShowsTerminalStates(t, ctx, store)
}

func expectMatchShowsTerminalStates(t *testing.T, ctx context.Context, store matchStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	stored, err := store.Assert(ctx, assertion(nil), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if err := store.Disable(ctx, stored.ID, platformScope, "usr_ana", "wrong", now); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	got, err := store.Match(ctx, matchFor(nil))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Own == nil {
		t.Fatal("own = nil, want the disabled memory shown rather than hidden")
	}
	if got.Own.Status != domain.MemoryDisabled {
		t.Errorf("status = %s, want the state that explains why Find is silent", got.Own.Status)
	}
}

/*
Shared memory is answered separately from the agent's own.

The two mean different things to whoever asked. Their own memory is theirs to
correct; shared memory is what every agent in the scope reads, and improving it
is an act taken against the shared row. Collapsing them into one answer would
put a correction of everybody's memory behind a button that says "correct
this".
*/
func TestMatch_sharedMemoryIsAnsweredApartFromTheAgentsOwn(t *testing.T) {
	t.Parallel()
	expectMatchSeparatesShared(t, context.Background(), memory.NewMemory())
}

func TestPostgresMatch_sharedMemoryIsAnsweredApartFromTheAgentsOwn(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMatchSeparatesShared(t, ctx, store)
}

func expectMatchSeparatesShared(t *testing.T, ctx context.Context, store matchStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	shared, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.AgentID = ""
	}), "usr_ana", "for everybody", now)
	if err != nil {
		t.Fatalf("Assert shared: %v", err)
	}

	got, err := store.Match(ctx, matchFor(nil))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Own != nil {
		t.Errorf("own = %+v, want nothing in the agent's own namespace", got.Own)
	}
	if got.Shared == nil || got.Shared.ID != shared.ID {
		t.Fatalf("shared = %+v, want the memory every agent reads", got.Shared)
	}

	// Asked as a shared question, nothing covers it but itself.
	asShared, err := store.Match(ctx, matchFor(func(in *memory.MatchInput) { in.AgentID = "" }))
	if err != nil {
		t.Fatalf("Match shared: %v", err)
	}
	if asShared.Shared != nil {
		t.Errorf("shared = %+v, want nothing covering shared memory but itself", asShared.Shared)
	}
	if asShared.Own == nil || asShared.Own.ID != shared.ID {
		t.Errorf("own = %+v, want the shared row answered as its own", asShared.Own)
	}
}

/*
A proposal nobody has decided yet is shown too.

Otherwise the person teaches a fact an agent already proposed, the accept later
merges the two, and the queue keeps an item whose reason for existing has
already been answered somewhere they could not see.
*/
func TestMatch_showsTheProposalNobodyHasDecided(t *testing.T) {
	t.Parallel()
	expectMatchShowsPending(t, context.Background(), memory.NewMemory())
}

func TestPostgresMatch_showsTheProposalNobodyHasDecided(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMatchShowsPending(t, ctx, store)
}

func expectMatchShowsPending(t *testing.T, ctx context.Context, store matchStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	out, err := store.Suggest(ctx, suggestion(nil), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	got, err := store.Match(ctx, matchFor(nil))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Pending == nil || got.Pending.ID != out.Suggestion.ID {
		t.Fatalf("pending = %+v, want the proposal waiting for somebody", got.Pending)
	}
	if got.Own != nil {
		t.Errorf("own = %+v, want nothing active yet", got.Own)
	}
}

// Nothing here is nothing to show, and the caller can tell.
func TestMatch_anIdentityNobodyHasTaught_answersNothing(t *testing.T) {
	t.Parallel()
	got, err := memory.NewMemory().Match(context.Background(), matchFor(nil))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Answered() {
		t.Errorf("match = %+v, want nothing to show", got)
	}
}
