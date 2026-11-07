package policy_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/testdb"
)

// The hash on every step promises that the rules a decision was made under can
// be fetched. These are the properties that make that promise true.

func storeFor(t *testing.T) *policy.Store {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `truncate policies, policy_snapshots`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return policy.NewStore(pool)
}

func rule(code string, over func(*domain.Policy)) domain.Policy {
	p := domain.Policy{
		Code: code, Name: "Sem exportação em massa", Owner: "Governança",
		Reason:   "exportações grandes saem por um canal revisado",
		Resource: "crm.*", Effects: []domain.Effect{domain.EffectWrite},
		Reach: domain.ReachInstallation, Effect: domain.PolicyDeny,
		Mode: domain.PolicyEnforce, Enabled: true,
		Conditions: []domain.Condition{
			{Field: "args.rows", Op: domain.OpGreaterThan, Value: "100"},
		},
	}
	if over != nil {
		over(&p)
	}
	return p
}

func TestPut_takesASnapshotThatCanBeFetchedBack(t *testing.T) {
	store := storeFor(t)

	set, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if set.Hash == "" {
		t.Fatal("no hash was returned")
	}

	// The whole point. A decision recorded under this hash can be explained
	// years later by reading the rules it was made under.
	back, err := store.Snapshot(t.Context(), set.Hash)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(back.Policies) != 1 || back.Policies[0].Code != "POL-114" {
		t.Errorf("snapshot = %+v, want the policy that was written", back.Policies)
	}
	if back.TakenAt.IsZero() {
		t.Error("the snapshot does not say when it was taken")
	}
}

func TestPut_changingAPolicy_leavesTheOldSnapshotReadable(t *testing.T) {
	store := storeFor(t)

	before, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	after, err := store.Put(t.Context(), rule("POL-114", func(p *domain.Policy) {
		p.Effect = domain.PolicyEscalate
	}), "usr_ana", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put again: %v", err)
	}

	if before.Hash == after.Hash {
		t.Fatal("changing a rule did not change the hash naming the set")
	}

	// A run decided before the change is explained by the rules of its time,
	// not by what somebody edited afterwards.
	old, err := store.Snapshot(t.Context(), before.Hash)
	if err != nil {
		t.Fatalf("the earlier snapshot is gone: %v", err)
	}
	if old.Policies[0].Effect != domain.PolicyDeny {
		t.Errorf("the earlier snapshot says %q, want the rule as it was", old.Policies[0].Effect)
	}
}

func TestPut_editingBackToWhatItWas_returnsToTheSameHash(t *testing.T) {
	store := storeFor(t)

	first, _ := store.Put(t.Context(), rule("POL-114", nil), "usr_ana", domain.IfMatch{})
	if _, err := store.Put(t.Context(), rule("POL-114", func(p *domain.Policy) {
		p.Effect = domain.PolicyAllow
	}), "usr_ana", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	back, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Correct rather than surprising: the rules in force are identical, so a
	// decision under either is the same decision, and the name should say so.
	if back.Hash != first.Hash {
		t.Errorf("hash = %s, want the one the identical set had (%s)", back.Hash, first.Hash)
	}
}

func TestActive_hashDoesNotMoveWhenNothingChanged(t *testing.T) {
	store := storeFor(t)

	if _, err := store.Put(t.Context(), rule("POL-200", nil), "usr_ana", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := store.Put(t.Context(), rule("POL-100", nil), "usr_ana", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// A hash that moved with row order would make every restart look like a
	// policy change, and seal every step to a name nobody can reproduce.
	first, err := store.Active(t.Context())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	second, err := store.Active(t.Context())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if first.Hash != second.Hash {
		t.Errorf("two reads of the same set gave %s and %s", first.Hash, second.Hash)
	}
}

func TestActive_anInstallationWithNoPolicies_stillHasAHash(t *testing.T) {
	store := storeFor(t)

	set, err := store.Active(t.Context())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	// "No rules" is a state a decision can be made under, and it has to be
	// nameable like any other.
	if set.Hash == "" {
		t.Error("an empty set has no hash")
	}
	if set.Policies == nil {
		t.Error("an empty set decoded as nil, which hashes differently from empty")
	}
}

func TestDelete_snapshotsWhatIsLeft(t *testing.T) {
	store := storeFor(t)

	if _, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	set, err := store.Delete(t.Context(), "POL-114", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(set.Policies) != 0 {
		t.Errorf("policies = %+v, want none left", set.Policies)
	}

	back, err := store.Snapshot(t.Context(), set.Hash)
	if err != nil {
		t.Fatalf("the snapshot after a delete is not readable: %v", err)
	}
	if len(back.Policies) != 0 {
		t.Errorf("snapshot = %+v, want the empty set", back.Policies)
	}
}

func TestSnapshot_unknownHash_isAnError(t *testing.T) {
	store := storeFor(t)

	// Never an empty set. A decision whose rules cannot be found must not be
	// explained as "there were no rules".
	if _, err := store.Snapshot(t.Context(), "pol_deadbeef"); !errors.Is(err, policy.ErrNoSnapshot) {
		t.Errorf("Snapshot of an unknown hash = %v, want ErrNoSnapshot", err)
	}
}

func TestPut_roundTripsEveryFieldTheGateReads(t *testing.T) {
	store := storeFor(t)

	written := rule("POL-114", func(p *domain.Policy) {
		p.Reach = domain.ReachScopes
		p.Scopes = []domain.Scope{{Company: "acme", Area: "cx"}}
		p.Agents = []domain.AgentID{"triage"}
		p.Mode = domain.PolicyMonitor
	})
	set, err := store.Put(t.Context(), written, "usr_ana", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// A field lost in storage is a rule that does something other than what
	// its author wrote, which is the failure this whole model exists to avoid.
	got := set.Policies[0]
	if got.Resource != written.Resource || len(got.Conditions) != 1 {
		t.Errorf("resource/conditions = %q/%v, want them back", got.Resource, got.Conditions)
	}
	if len(got.Effects) != 1 || got.Effects[0] != domain.EffectWrite {
		t.Errorf("effects = %v, want write", got.Effects)
	}
	if got.Reach != domain.ReachScopes || len(got.Scopes) != 1 || got.Scopes[0].Area != "cx" {
		t.Errorf("reach = %v %v, want the scope it was given", got.Reach, got.Scopes)
	}
	if got.Mode != domain.PolicyMonitor {
		t.Errorf("mode = %q, want monitor — a rule enforcing when it should watch", got.Mode)
	}
	// Everything, not a proxy for it. This used to compare the two rendered
	// sentences, which was a neat shorthand and a weak one: the sentence never
	// carried the reason, the owner or whether the rule was enabled, so a
	// field lost in storage could pass it.
	if !reflect.DeepEqual(got, written) {
		t.Errorf("read back a different rule:\n  stored:  %+v\n  written: %+v", got, written)
	}
}

func TestActive_hashWithNoSnapshotBehindIt_takesOne(t *testing.T) {
	store := storeFor(t)

	if _, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Anything that touches the table without going through Put — a
	// migration, a support script, somebody with psql — leaves a set whose
	// hash names no snapshot. The Gate would then seal every decision to a
	// name an auditor cannot fetch, which is the one promise this package
	// exists to keep.
	if _, err := store.Exec(t.Context(),
		`update policies set mode = 'enforce' where code = 'POL-114'`); err != nil {
		t.Fatalf("raw update: %v", err)
	}

	set, err := store.Active(t.Context())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if _, err := store.Snapshot(t.Context(), set.Hash); err != nil {
		t.Errorf("the hash Active reported has no snapshot behind it: %v", err)
	}
}

func mustIfMatch(t *testing.T, header string) domain.IfMatch {
	t.Helper()
	parsed, err := domain.ParseIfMatch(&header)
	if err != nil {
		t.Fatalf("ParseIfMatch: %v", err)
	}
	return parsed
}

func TestRevisionOf_absentAndEmptyListsAreTheSameRule(t *testing.T) {
	t.Parallel()
	bare := rule("POL-1", nil)
	bare.Effects, bare.Scopes, bare.Agents, bare.Conditions = nil, nil, nil, nil
	empty := bare
	empty.Effects, empty.Scopes, empty.Agents, empty.Conditions =
		[]domain.Effect{}, []domain.Scope{}, []domain.AgentID{}, []domain.Condition{}
	if policy.RevisionOf(bare) != policy.RevisionOf(empty) {
		t.Fatal("the same rule has two revisions")
	}
}

func TestPut_whatIsAlreadyStored_changesNothing(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()
	full := rule("POL-1", func(p *domain.Policy) {
		p.Effects = []domain.Effect{domain.EffectWrite}
		p.Conditions = []domain.Condition{{Field: "amount", Op: domain.OpEquals, Value: "10"}}
		p.Reach, p.Agents = domain.ReachAgents, []domain.AgentID{"a"}
	})
	first, err := store.Put(ctx, full, "usr_a", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !first.Changed || first.Revision == "" {
		t.Fatalf("first write = %+v", first)
	}
	before := policyRowState(t, "POL-1")

	again, err := store.Put(ctx, full, "usr_b", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put again: %v", err)
	}

	if again.Changed || again.Revision != first.Revision || again.Hash != first.Hash {
		t.Fatalf("again = %+v, first = %+v", again, first)
	}
	if after := policyRowState(t, "POL-1"); after != before {
		t.Fatalf("the row moved: %s, then %s", before, after)
	}
}

// updated_at, updated_by and the count of snapshots: what a real write moves.
func policyRowState(t *testing.T, code string) string {
	t.Helper()
	var state string
	if err := testdb.Pool(t).QueryRow(context.Background(), `
		select p.updated_at::text || '|' || p.updated_by || '|' ||
		       (select count(*) from policy_snapshots)::text
		from policies p where p.code = $1`, code).Scan(&state); err != nil {
		t.Fatalf("read state: %v", err)
	}
	return state
}

// A rule a snapshot does not name would be enforced under a hash that cannot be fetched,
// and the no-op must not depend on the write that would have taken it.
func TestPut_noChangeOnASetWithNoSnapshot_stillAnswersItsHash(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()
	p := rule("POL-1", nil)
	if _, err := store.Put(ctx, p, "usr_a", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := store.Exec(ctx, `delete from policy_snapshots`); err != nil {
		t.Fatalf("clear: %v", err)
	}

	again, err := store.Put(ctx, p, "usr_a", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	active, err := store.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if again.Hash != active.Hash {
		t.Fatalf("hash = %s, active = %s", again.Hash, active.Hash)
	}
	if _, err := store.Snapshot(ctx, again.Hash); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
}

func TestPut_ifMatch_decidesOnWhatIsThere(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()
	first, err := store.Put(ctx, rule("POL-1", nil), "usr_a", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	changed := rule("POL-1", func(p *domain.Policy) { p.Name = "changed" })

	var stale *domain.StaleError
	_, err = store.Put(ctx, changed, "usr_a", mustIfMatch(t, `"v1-old"`))
	if !errors.As(err, &stale) || stale.Current != first.Revision {
		t.Fatalf("stale write = %v", err)
	}
	if got, _ := store.Named(ctx, "POL-1"); got.Name == "changed" {
		t.Fatal("a refused write was stored")
	}

	_, err = store.Put(ctx, rule("POL-NEW", nil), "usr_a", mustIfMatch(t, `*`))
	if !errors.As(err, &stale) || stale.Current != "" {
		t.Fatalf("star on nothing = %v", err)
	}
	if _, err := store.Named(ctx, "POL-NEW"); !errors.Is(err, policy.ErrNoPolicy) {
		t.Fatalf("it was created: %v", err)
	}

	written, err := store.Put(ctx, changed, "usr_a", mustIfMatch(t, string(first.Revision)))
	if err != nil || !written.Changed || written.Revision == first.Revision {
		t.Fatalf("a current write = %+v, %v", written, err)
	}
}

// If-Match is judged before the no-op: a client that named an old revision is told so,
// even if what it sent happens to be what is there.
func TestPut_staleButIdentical_isStillRefused(t *testing.T) {
	store := storeFor(t)
	p := rule("POL-1", nil)
	if _, err := store.Put(context.Background(), p, "usr_a", domain.IfMatch{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	var stale *domain.StaleError
	if _, err := store.Put(context.Background(), p, "usr_a", mustIfMatch(t, `"v1-old"`)); !errors.As(err, &stale) {
		t.Fatalf("Put = %v, want it refused", err)
	}
}

func TestDelete_ifMatch_decidesOnWhatIsThere(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()
	first, err := store.Put(ctx, rule("POL-1", nil), "usr_a", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	var stale *domain.StaleError
	if _, err := store.Delete(ctx, "POL-1", mustIfMatch(t, `"v1-old"`)); !errors.As(err, &stale) {
		t.Fatalf("stale delete = %v", err)
	}
	if _, err := store.Named(ctx, "POL-1"); err != nil {
		t.Fatalf("a refused delete removed it: %v", err)
	}
	if _, err := store.Delete(ctx, "POL-NOBODY", mustIfMatch(t, `*`)); !errors.As(err, &stale) {
		t.Fatalf("star on nothing = %v", err)
	}
	if _, err := store.Delete(ctx, "POL-1", mustIfMatch(t, string(first.Revision))); err != nil {
		t.Fatalf("a current delete = %v", err)
	}
	if _, err := store.Named(ctx, "POL-1"); !errors.Is(err, policy.ErrNoPolicy) {
		t.Fatalf("Named = %v", err)
	}
}

/*
overlapped runs the writers so that every one of them has read before any can write.

Started together they do not overlap: opening a connection takes far longer than a write,
so they finish one after another and a missing lock goes unseen. So this holds the row.
A plain read passes that hold and a write does not, so without the guard each writer reads
the old revision, passes, and waits at its write; with it the first does that and the rest
wait at the guard. Either way, every writer is waiting on a lock before the row is let go.
*/
func overlapped(t *testing.T, code string, writers []func() error) []error {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Pool(t)
	hold, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = hold.Rollback(ctx) }()
	if _, err := hold.Exec(ctx, `select 1 from policies where code = $1 for update`, code); err != nil {
		t.Fatalf("hold: %v", err)
	}

	results := make(chan error, len(writers))
	for _, write := range writers {
		go func() { results <- write() }()
	}
	deadline := time.Now().Add(20 * time.Second)
	for waiting := 0; waiting < len(writers); {
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d writers reached a lock", waiting, len(writers))
		}
		if err := pool.QueryRow(ctx, `
			select count(*) from pg_stat_activity
			where datname = current_database() and wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("watch: %v", err)
		}
	}
	if err := hold.Rollback(ctx); err != nil {
		t.Fatalf("release: %v", err)
	}

	out := make([]error, 0, len(writers))
	for range writers {
		out = append(out, <-results)
	}
	return out
}

func tally(t *testing.T, results []error) (won, refused int) {
	t.Helper()
	for _, err := range results {
		var stale *domain.StaleError
		switch {
		case err == nil:
			won++
		case errors.As(err, &stale):
			refused++
		default:
			t.Fatalf("write: %v", err)
		}
	}
	return won, refused
}

// The race the header exists for. Without the guard, both read the same revision, both
// pass, and the second silently replaces the first.
func TestPut_twoWritersWithOneRevision_exactlyOneWins(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()
	first, err := store.Put(ctx, rule("POL-1", nil), "usr_a", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	expect := mustIfMatch(t, string(first.Revision))

	const count = 6
	writers := make([]func() error, count)
	for i := range writers {
		writers[i] = func() error {
			_, err := store.Put(ctx, rule("POL-1", func(p *domain.Policy) {
				p.Name = fmt.Sprintf("writer %d", i)
			}), "usr_a", expect)
			return err
		}
	}

	won, refused := tally(t, overlapped(t, "POL-1", writers))
	if won != 1 || refused != count-1 {
		t.Fatalf("won %d and refused %d of %d", won, refused, count)
	}
}

// Once the row is gone there is nothing left to lock. A writer that read it before the
// delete would otherwise bring back a rule somebody just removed.
func TestPut_racingADelete_exactlyOneWins(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()
	first, err := store.Put(ctx, rule("POL-1", nil), "usr_a", domain.IfMatch{})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	expect := mustIfMatch(t, string(first.Revision))

	won, refused := tally(t, overlapped(t, "POL-1", []func() error{
		func() error {
			_, err := store.Delete(ctx, "POL-1", expect)
			return err
		},
		func() error {
			_, err := store.Put(ctx, rule("POL-1", func(p *domain.Policy) { p.Name = "changed" }), "usr_a", expect)
			return err
		},
	}))
	if won != 1 || refused != 1 {
		t.Fatalf("won %d and refused %d", won, refused)
	}
}
