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
