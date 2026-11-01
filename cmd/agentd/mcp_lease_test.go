package main

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
)

// recordingLeases answers every renewal with lapsed, and remembers what it
// was asked to hold.
type recordingLeases struct {
	lapsed bool
	calls  [][]string
	holder string
}

func (l *recordingLeases) HoldToolServers(_ context.Context, holder string, servers []string) (bool, error) {
	l.holder = holder
	l.calls = append(l.calls, servers)
	return l.lapsed, nil
}

// leasingReconciler is a reconciler holding the command-line server "local"
// as worker-a, over a configuration with nothing in it.
func leasingReconciler(t *testing.T, lapsed bool) (*reconciler, *recordingPublisher, *recordingLeases) {
	t.Helper()
	r, publisher := reconcilerOver(t, nil, false)
	if err := r.catalog.AddServer(context.Background(), "local",
		&testSession{tools: []*mcp.Tool{{Name: "run"}}}, nil); err != nil {
		t.Fatalf("connect the command-line server: %v", err)
	}
	r.hold(domain.MCPServer{Name: "local"})
	leases := &recordingLeases{lapsed: lapsed}
	r.holdingAs("worker-a", leases)
	return r, publisher, leases
}

// Every pass renews the lease on the servers the flags started, in this
// worker's name.
func TestReconcile_aCommandLineServer_isLeasedOnEveryPass(t *testing.T) {
	t.Parallel()
	r, _, leases := leasingReconciler(t, false)

	r.reconcile(context.Background())
	r.reconcile(context.Background())

	want := [][]string{{"local"}, {"local"}}
	if leases.holder != "worker-a" || !slices.EqualFunc(leases.calls, want, slices.Equal) {
		t.Fatalf("renewed %v as %q, want %v as worker-a", leases.calls, leases.holder, want)
	}
}

// A worker holding no command-line server still passes through the leases:
// its pass is what sweeps the servers every worker dropped.
func TestReconcile_noCommandLineServer_stillPassesThroughTheLeases(t *testing.T) {
	t.Parallel()
	r, _ := reconcilerOver(t, nil, false)
	leases := &recordingLeases{}
	r.holdingAs("worker-b", leases)

	r.reconcile(context.Background())

	if len(leases.calls) != 1 || len(leases.calls[0]) != 0 {
		t.Fatalf("renewed %v, want one pass holding nothing", leases.calls)
	}
}

// A lease that had run out may have let the server's tools be swept. With
// nothing new connected the pass would publish nothing, so the lapse itself
// is the reason to publish — in this worker's name.
func TestReconcile_aLeaseThatHadRunOut_publishesWithNothingNew(t *testing.T) {
	t.Parallel()
	r, publisher, _ := leasingReconciler(t, true)

	r.reconcile(context.Background())

	if publisher.calls != 1 || publisher.last.Holder != "worker-a" {
		t.Fatalf("published %d times as %q, want once as worker-a", publisher.calls, publisher.last.Holder)
	}
}

// A live lease is not news: a pass with nothing new takes no lock to say so.
func TestReconcile_aLiveLease_publishesNothing(t *testing.T) {
	t.Parallel()
	r, publisher, _ := leasingReconciler(t, false)

	r.reconcile(context.Background())

	if publisher.calls != 0 {
		t.Fatalf("published %d times with nothing new and the lease live", publisher.calls)
	}
}

// localFlag is the command-line server the dead-session tests hold.
var localFlag = domain.MCPServer{
	Name: "local", Transport: domain.TransportStdio, Command: "local-server", AcceptsLocalExecution: true,
}

// deadFlagReconciler holds the command-line server "local" as worker-a, whose
// session has died: the flag still names it, the catalogue has no session.
// Its reconnect answers as told.
func deadFlagReconciler(t *testing.T, reconnects bool) (*reconciler, *recordingPublisher, *recordingLeases) {
	t.Helper()
	r, publisher := reconcilerOver(t, nil, !reconnects)
	r.hold(localFlag)
	leases := &recordingLeases{}
	r.holdingAs("worker-a", leases)
	return r, publisher, leases
}

// A session that died is not a server this worker serves. Renewing its lease
// would keep its tools in the catalogue for as long as the worker runs —
// the ghost tools #223 is about.
func TestReconcile_aDeadCommandLineSessionThatCannotReconnect_isNotLeased(t *testing.T) {
	t.Parallel()
	r, _, leases := deadFlagReconciler(t, false)

	r.reconcile(context.Background())

	if len(leases.calls) != 1 || len(leases.calls[0]) != 0 {
		t.Fatalf("renewed %v, want one pass holding nothing while local's session is dead", leases.calls)
	}
}

// The flag still names the server, so the pass reconnects it — and only then
// leases it again, and publishes what it now offers.
func TestReconcile_aDeadCommandLineSession_isReconnectedThenLeased(t *testing.T) {
	t.Parallel()
	r, publisher, leases := deadFlagReconciler(t, true)

	r.reconcile(context.Background())

	if !r.catalog.Connected("local") || !slices.EqualFunc(leases.calls, [][]string{{"local"}}, slices.Equal) ||
		publisher.calls != 1 {
		t.Fatalf("connected %v, renewed %v, published %d times; want reconnected, leased and published once",
			r.catalog.Connected("local"), leases.calls, publisher.calls)
	}
}

// Held but dead, the server is not restated as reachable: the console would
// otherwise show a live integration where there is only the ghost of one.
func TestReconcile_aDeadCommandLineSession_isNotReportedReachable(t *testing.T) {
	t.Parallel()
	r, _, _ := deadFlagReconciler(t, false)

	r.reconcile(context.Background())

	seen, observed := r.health.(recordingHealth)["local"]
	if !observed || seen.Reachable {
		t.Fatalf("local observed %v as %+v, want it observed unreachable", observed, seen)
	}
}
