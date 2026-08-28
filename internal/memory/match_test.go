package memory_test

import (
	"context"
	"errors"
	"fmt"
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
A row written before the canonical key is found under a different spelling too.

The lookup matches the key or the raw assertion id, and a legacy row has neither
that answers to a new spelling — so the person most likely to be told "already
here" by the merge a moment later was the one told "nothing here" by the screen.
The write path has compared identities in Go since lazy keying; the read had
not.
*/
func TestPostgresMatch_aLegacyRowUnderAnotherSpelling_isFound(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	stored, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.Subject = "Grafana Datasource"
	}), "usr_ana", "reviewed", now)
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	unkey(t, ctx, pool, stored.ID)

	got, err := store.Match(ctx, matchFor(func(in *memory.MatchInput) {
		in.Subject = "grafana  datasource"
	}))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Own == nil || got.Own.ID != stored.ID {
		t.Fatalf("own = %+v, want the keyless row found by its identity", got.Own)
	}
	// And still keyless: this is the read a screen makes on every keystroke, and
	// keying a row is a write.
	if hasKey(t, ctx, pool, stored.ID) {
		t.Error("the match repaired the row it was only asked about")
	}
}

/*
Two keyless rows of one identity are refused here too, not chosen between.

The write path has refused this since the canonical conflict landed. A read that
answered anyway would show whichever sorted first, which is half the problem
presented as the whole of it — and the person would correct one row while the
other went on saying something else.
*/
func TestPostgresMatch_twoLegacyRowsOfOneIdentity_refuseToChoose(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	for i, subject := range []string{"grafana  datasource", "GRAFANA DATASOURCE"} {
		written, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
			a.Subject = fmt.Sprintf("grafana datasource %d", i)
		}), "usr_ana", "reviewed", now)
		if err != nil {
			t.Fatalf("Assert %q: %v", subject, err)
		}
		legacyTwin(t, ctx, pool, written.ID, subject)
	}

	if _, err := store.Match(ctx, matchFor(func(in *memory.MatchInput) {
		in.Subject = "Grafana Datasource"
	})); !errors.Is(err, memory.ErrCanonicalConflict) {
		t.Fatalf("Match = %v, want the pair refused rather than one of them shown", err)
	}
}

/*
A proposal is found by identity, not by the id its own spelling hashes to.

Otherwise somebody teaches a fact an agent proposed an hour ago in slightly
different words, the accept later merges the two, and the queue keeps an item
whose reason for existing was answered somewhere they could not see.
*/
func TestMatch_aProposalUnderAnotherSpelling_isFound(t *testing.T) {
	t.Parallel()
	expectMatchFindsAnotherSpellingOfAProposal(t, context.Background(), memory.NewMemory())
}

func TestPostgresMatch_aProposalUnderAnotherSpelling_isFound(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMatchFindsAnotherSpellingOfAProposal(t, ctx, store)
}

func expectMatchFindsAnotherSpellingOfAProposal(
	t *testing.T, ctx context.Context, store matchStore,
) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	out, err := store.Suggest(ctx, suggestion(func(s *domain.MemorySuggestion) {
		s.Subject = "Grafana Datasource"
	}), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	got, err := store.Match(ctx, matchFor(func(in *memory.MatchInput) {
		in.Subject = "grafana  datasource"
	}))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Pending == nil || got.Pending.ID != out.Suggestion.ID {
		t.Fatalf("pending = %+v, want the proposal found by its identity", got.Pending)
	}
}

/*
A proposal is recorded with its canonical key, so the index can serve the match.

The unkeyed fallback would find it either way, which is exactly why this needs
its own accuser: a column that stops being written keeps working, slowly, by
scanning every pending row in the namespace on every keystroke — and nothing
fails until somebody has enough of them for it to matter.
*/
func TestPostgresSuggest_recordsTheCanonicalIdentityKey(t *testing.T) {
	ctx, pool := postgresPool(t)
	store := memory.NewPostgres(pool)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	out, err := store.Suggest(ctx, suggestion(nil), reviewPolicy, "agent:triage", now)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	var key *string
	if err := pool.QueryRow(ctx,
		`select canonical_identity_key from memory_suggestions where suggestion_id = $1`,
		out.Suggestion.ID).Scan(&key); err != nil {
		t.Fatalf("read the key: %v", err)
	}
	if key == nil {
		t.Fatal("the proposal was recorded without its key, so every match scans for it")
	}
	if *key != domain.CanonicalIdentityKey(domain.MemoryAssertion{
		Scope: out.Suggestion.Scope, AgentID: out.Suggestion.AgentID,
		Kind: out.Suggestion.Kind, Subject: out.Suggestion.Subject,
		Signature: out.Suggestion.Signature,
	}) {
		t.Errorf("key = %q, want the identity the assertions are keyed by", *key)
	}
}

/*
An expired memory says expired.

Expiry is a moment passing rather than something anybody did, so it is stored
active and projected. A match that returned the stored value would tell somebody
their memory is active while the screen beside it shows nothing — which is the
exact confusion this exists to end. The other terminal states are stored as
themselves and need no projection; disabled is covered above.
*/
func TestMatch_anExpiredMemorySaysExpired(t *testing.T) {
	t.Parallel()
	expectMatchProjectsTerminalStates(t, context.Background(), memory.NewMemory())
}

func TestPostgresMatch_anExpiredMemorySaysExpired(t *testing.T) {
	ctx, store := postgresStore(t)
	expectMatchProjectsTerminalStates(t, ctx, store)
}

func expectMatchProjectsTerminalStates(t *testing.T, ctx context.Context, store matchStore) {
	t.Helper()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	gone := now.Add(-time.Hour)
	if _, err := store.Assert(ctx, assertion(func(a *domain.MemoryAssertion) {
		a.ExpiresAt = &gone
	}), "usr_ana", "reviewed", now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("Assert: %v", err)
	}

	got, err := store.Match(ctx, matchFor(nil))
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if got.Own == nil {
		t.Fatal("own = nil, want the expired memory shown rather than hidden")
	}
	if got.Own.Status != domain.MemoryExpired {
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
