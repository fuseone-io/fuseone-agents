package admin_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

/*
A server started with --mcp has no configuration row, so nothing there can
say it is gone (#223). What says it is still served is a lease: every worker
that runs it renews one, and its tools leave when the last lease runs out.
*/

// leaseClock is the time every lease is written and judged at, moved by
// hand.
type leaseClock struct{ now time.Time }

func (c *leaseClock) read() time.Time { return c.now }

func (c *leaseClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// leasedCatalogue is a catalogue whose Curator reads the given clock.
func leasedCatalogue(t *testing.T) (*admin.Curator, *leaseClock) {
	t.Helper()
	curator, _ := catalogueOf(t)
	clock := &leaseClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	return curator.WithClock(clock.read), clock
}

var localRun = []domain.ToolEntry{{ID: "local.run", Server: "local"}}

func renew(t *testing.T, curator *admin.Curator, holder string, servers ...string) bool {
	t.Helper()
	lapsed, err := curator.HoldToolServers(context.Background(), holder, servers)
	if err != nil {
		t.Fatalf("HoldToolServers(%s, %v): %v", holder, servers, err)
	}
	return lapsed
}

// The worker that ran it restarted without the flag, and nobody renews the
// lease its publication took. Any worker's next pass takes the tools away —
// without a publication, because a worker with nothing new publishes nothing.
func TestHoldToolServers_anUnmanagedServerNobodyHolds_losesItsTools(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	publish(t, curator, standalone(localRun))

	clock.advance(admin.ToolServerLease + time.Second)
	renew(t, curator, "worker-b")

	if got := offered(t, curator, "local"); len(got) != 0 {
		t.Fatalf("local offers %v after every lease on it ran out", got)
	}
}

// Before the lease runs out the server is still served, and the sweep must
// not take what a live worker offers.
func TestHoldToolServers_aLeaseNotYetRunOut_keepsTheTools(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	publish(t, curator, standalone(localRun))

	clock.advance(admin.ToolServerLease - time.Second)
	renew(t, curator, "worker-b")

	if got := offered(t, curator, "local"); !slices.Equal(got, []domain.ToolID{"local.run"}) {
		t.Fatalf("local offers %v while its lease is live, want its tool kept", got)
	}
}

// A worker that keeps running the server keeps it offered for as long as it
// runs, well past the lease its first publication took.
func TestHoldToolServers_renewedEveryPass_keepsTheToolsPastTheFirstLease(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	publish(t, curator, standalone(localRun))

	for range 4 {
		clock.advance(admin.ToolServerLease / 2)
		renew(t, curator, "worker-a", "local")
	}

	if got := offered(t, curator, "local"); !slices.Equal(got, []domain.ToolID{"local.run"}) {
		t.Fatalf("local offers %v though its worker renewed every pass", got)
	}
}

// Two workers run the server; one drops the flag. The other still serves it,
// so its tools stay until that worker stops renewing too.
func TestHoldToolServers_oneOfTwoHoldersStops_theOtherKeepsTheTools(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	publish(t, curator, standalone(localRun))
	renew(t, curator, "worker-b", "local")

	clock.advance(admin.ToolServerLease / 2)
	renew(t, curator, "worker-b", "local")
	clock.advance(admin.ToolServerLease/2 + time.Second)
	renew(t, curator, "worker-a")

	if got := offered(t, curator, "local"); !slices.Equal(got, []domain.ToolID{"local.run"}) {
		t.Fatalf("local offers %v after worker-a stopped, though worker-b still serves it", got)
	}
}

// A worker learns that its lease had run out — or never existed — so it
// publishes again: its server's tools may have been swept meanwhile, and a
// worker with nothing new would otherwise never put them back.
func TestHoldToolServers_aLeaseThatHadRunOut_isReportedAsLapsed(t *testing.T) {
	curator, clock := leasedCatalogue(t)

	first := renew(t, curator, "worker-a", "local")
	clock.advance(admin.ToolServerLease / 2)
	renewed := renew(t, curator, "worker-a", "local")
	clock.advance(admin.ToolServerLease + time.Second)
	afterGap := renew(t, curator, "worker-a", "local")

	if !first || renewed || !afterGap {
		t.Fatalf("lapsed = %v, %v, %v; want true for a new lease, false for a renewal, true after a gap",
			first, renewed, afterGap)
	}
}

// A publication is a pass too: it sweeps the servers nobody holds, as it
// sweeps the servers the configuration no longer wants.
func TestPublish_anUnmanagedServerWhoseLeaseRanOut_isSwept(t *testing.T) {
	curator, pool := catalogueOf(t)
	clock := &leaseClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	curator = curator.WithClock(clock.read)
	crm := configure(t, pool, "crm", true)
	publish(t, curator, standalone(localRun))

	clock.advance(admin.ToolServerLease + time.Second)
	publish(t, curator, discovered("crm", crm, "crm.lookup"))

	if got := offered(t, curator, "local"); len(got) != 0 {
		t.Fatalf("local offers %v after a publication swept a lease that ran out", got)
	}
}

// An unmanaged server is recorded only under a lease. A publication that
// names no holder would leave rows nobody is accountable for renewing.
func TestPublish_anUnmanagedServerWithNoHolder_isRefused(t *testing.T) {
	curator, _ := leasedCatalogue(t)
	pub := standalone(localRun)
	pub.Holder = ""

	if err := curator.Publish(context.Background(), pub); err == nil {
		t.Fatal("a publication of an unmanaged server with no holder was recorded")
	}
	if got := offered(t, curator, "local"); len(got) != 0 {
		t.Fatalf("local offers %v from a publication with no holder", got)
	}
}

// The tools leave; the ruling on them stays, as it does for a deleted
// server. The same server started again offers the tool already ruled on.
func TestHoldToolServers_aSweptServerStartedAgain_findsItsRuling(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	run := domain.ToolEntry{ID: "local.run", Server: "local", Digest: "sha-run"}
	publish(t, curator, standalone([]domain.ToolEntry{run}))
	if err := curator.Classify(context.Background(), domain.Scope{}, domain.ToolClassification{
		Tool: run.ID, Effect: domain.EffectWrite, By: "usr_ana", Digest: run.Digest,
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	clock.advance(admin.ToolServerLease + time.Second)
	renew(t, curator, "worker-b")
	if got := offered(t, curator, "local"); len(got) != 0 {
		t.Fatalf("local offers %v; the sweep below would prove nothing", got)
	}

	publish(t, curator, standalone([]domain.ToolEntry{run}))

	entries, err := curator.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got := find(t, entries, run.ID); got.Effect != domain.EffectWrite || got.Stale {
		t.Fatalf("local.run reads %+v, want the write ruling it had", got)
	}
}

// writtenByAnOldWorker records a tool the way a worker from before leases
// publishes an unmanaged server: marked unmanaged, and nothing else. That
// worker may be alive and serving it, and will never renew a lease it does
// not know exists.
func writtenByAnOldWorker(t *testing.T, server, id string) {
	t.Helper()
	if _, err := openPool(t).Exec(context.Background(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_at)
		values ('installation', '', '', 'tool', $1,
		        jsonb_build_object('server', $2::text, 'onSurface', false, 'unmanaged', true), true, now())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set value = excluded.value`,
		id, server); err != nil {
		t.Fatalf("write %s as an old worker would: %v", id, err)
	}
}

/*
A new worker adopts an old worker's server: it publishes it under a lease, so
the old rows are now its own and the grace they had is settled. A day later an
old worker publishes the server again, the old way. That is a new old-style
publication, and it gets a grace of its own — not the one the adoption
settled, long over by now, which would sweep it at once.
*/
func TestPublish_anAdoptedServerPublishedTheOldWayAgain_getsAGraceOfItsOwn(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	writtenByAnOldWorker(t, "old", "old.run")
	renew(t, curator, "worker-new")
	publish(t, curator, domain.ToolPublication{
		Servers: []domain.PublishedServer{{Name: "old", Unmanaged: true}},
		Entries: []domain.ToolEntry{{ID: "old.run", Server: "old"}}, Holder: "worker-a",
	})

	clock.advance(admin.LegacyToolGrace)
	renew(t, curator, "worker-a", "old")
	writtenByAnOldWorker(t, "old", "old.run")
	clock.advance(admin.ToolServerLease + time.Second)
	renew(t, curator, "worker-new")

	if got := offered(t, curator, "old"); !slices.Equal(got, []domain.ToolID{"old.run"}) {
		t.Fatalf("old offers %v; the old worker's new publication was swept on the adoption's expired grace", got)
	}
}

/*
A rolling upgrade: an old worker still serves a --mcp server, published the
old way with no lease. A new worker that does not run that server sweeps. The
old worker's tools get one grace period from the first sweep that sees them
rather than leaving at once — and once it is over, with nobody renewing, they
leave like any other unheld server's.
*/
func TestHoldToolServers_anOldWorkersUnleasedServer_survivesTheGraceThenLeaves(t *testing.T) {
	curator, clock := leasedCatalogue(t)
	writtenByAnOldWorker(t, "old", "old.run")

	renew(t, curator, "worker-new")
	clock.advance(admin.LegacyToolGrace - time.Second)
	renew(t, curator, "worker-new")
	if got := offered(t, curator, "old"); !slices.Equal(got, []domain.ToolID{"old.run"}) {
		t.Fatalf("old offers %v inside the grace, while the old worker may still serve it", got)
	}

	clock.advance(2 * time.Second)
	renew(t, curator, "worker-new")

	if got := offered(t, curator, "old"); len(got) != 0 {
		t.Fatalf("old offers %v after the grace ran out with nobody renewing", got)
	}
}

// A publication sweeps too, and gives an old worker's server the same grace.
func TestPublish_anOldWorkersUnleasedServer_isGivenTheGraceToo(t *testing.T) {
	curator, pool := catalogueOf(t)
	clock := &leaseClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	curator = curator.WithClock(clock.read)
	crm := configure(t, pool, "crm", true)
	writtenByAnOldWorker(t, "old", "old.run")

	publish(t, curator, discovered("crm", crm, "crm.lookup"))
	clock.advance(admin.LegacyToolGrace - time.Second)
	publish(t, curator, discovered("crm", crm, "crm.lookup"))
	kept := offered(t, curator, "old")
	clock.advance(2 * time.Second)
	publish(t, curator, discovered("crm", crm, "crm.lookup"))

	if left := offered(t, curator, "old"); !slices.Equal(kept, []domain.ToolID{"old.run"}) || len(left) != 0 {
		t.Fatalf("old offered %v inside the grace and %v after; want kept, then gone", kept, left)
	}
}
