package policy_test

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
)

// The hash on every step promises that the rules a decision was made under can
// be fetched. These are the properties that make that promise true.

func storeFor(t *testing.T) *policy.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the policy suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
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

	set, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana")
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

	before, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	after, err := store.Put(t.Context(), rule("POL-114", func(p *domain.Policy) {
		p.Effect = domain.PolicyEscalate
	}), "usr_ana")
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

	first, _ := store.Put(t.Context(), rule("POL-114", nil), "usr_ana")
	if _, err := store.Put(t.Context(), rule("POL-114", func(p *domain.Policy) {
		p.Effect = domain.PolicyAllow
	}), "usr_ana"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	back, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana")
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

	if _, err := store.Put(t.Context(), rule("POL-200", nil), "usr_ana"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := store.Put(t.Context(), rule("POL-100", nil), "usr_ana"); err != nil {
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

	if _, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	set, err := store.Delete(t.Context(), "POL-114")
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
	set, err := store.Put(t.Context(), written, "usr_ana")
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

	if _, err := store.Put(t.Context(), rule("POL-114", nil), "usr_ana"); err != nil {
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
