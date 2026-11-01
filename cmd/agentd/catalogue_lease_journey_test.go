package main

import (
	"context"
	"flag"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/known"
	"github.com/fuseone/agents/internal/tools"
)

/*
A server started with --mcp leaves the catalogue when it leaves every worker
(#223).

Its tools were published as unmanaged, so the sweep that clears removed
servers left them alone — and once every worker restarted without the flag,
nothing ever took them away. They stayed offered, and publishable.

These start where an operator does: the worker's flags, the worker's start-up
and its reconcile passes, and the console's tool list and classify route.
*/

// stdioToolServerArg tells the test binary, started again as a worker's
// --mcp command, to be that server instead of running tests.
const stdioToolServerArg = "stdio-tool-server"

// TestStdioToolServer is not a test. It is the --mcp server the journeys
// below start: the test binary, run again with an argument no test run
// passes, serving over stdio until the worker closes it.
//
// Given a path after the argument, it is a server that can die for good:
// exit_now leaves the path behind and exits, and a server started again
// finds the path and refuses to start.
func TestStdioToolServer(t *testing.T) {
	args := flag.Args()
	at := slices.Index(args, stdioToolServerArg)
	if at < 0 {
		t.Skip("only run as a --mcp command by the journeys in this file")
	}
	tombstone := ""
	if at+1 < len(args) {
		tombstone = args[at+1]
	}
	if _, err := os.Stat(tombstone); tombstone != "" && err == nil {
		os.Exit(1)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "local-reports", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "run_report"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "exit_now"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			if tombstone != "" {
				_ = os.WriteFile(tombstone, nil, 0o600)
			}
			os.Exit(0)
			return nil, nil, nil
		})
	_ = server.Run(context.Background(), &mcp.StdioTransport{})
	os.Exit(0)
}

// localReportsFlag is the --mcp flag that starts the server above; a
// tombstone path makes it one that cannot be started again once it died.
func localReportsFlag(tombstone ...string) string {
	flag := "local-reports=" + os.Args[0] + " -test.run=^TestStdioToolServer$ -- " + stdioToolServerArg
	for _, path := range tombstone {
		flag += " " + path
	}
	return flag
}

// journeyClock is the time every worker's leases are written and judged by:
// one installation, one clock, moved by hand. Read from the workers' watch
// loops while the test moves it.
type journeyClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *journeyClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *journeyClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// runningWorker is a worker process over the journey's database: started
// from its flags through connectTools, and passing whenever the test ticks.
type runningWorker struct {
	ticks   chan time.Time
	stop    context.CancelFunc
	catalog *tools.Catalog
}

// kill calls exit_now through the worker's catalogue, the way a run would
// call a tool, and keeps calling until the worker has noticed the session
// is gone.
func (w *runningWorker) kill(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	_, _ = w.catalog.Invoke(ctx, engine.Call{RunID: "run-kill", Seq: 1, Tool: "local-reports.exit_now"})
	deadline := time.Now().Add(10 * time.Second)
	for seq := int64(2); w.catalog.Connected("local-reports"); seq++ {
		if time.Now().After(deadline) {
			t.Fatal("the worker never noticed local-reports died; nothing below would prove anything")
		}
		_, _ = w.catalog.Invoke(ctx, engine.Call{RunID: "run-kill", Seq: seq, Tool: localRunReport})
		time.Sleep(10 * time.Millisecond)
	}
}

// pass returns once the worker has completed at least one whole reconcile
// pass after it was called. The watch takes a tick only between passes, so
// the second tick is taken only after the pass the first one started.
func (w *runningWorker) pass() {
	w.ticks <- time.Time{}
	w.ticks <- time.Time{}
}

// startWorker runs a worker the way workerCmd does, from these flags.
func (j *factoryJourney) startWorker(t *testing.T, clock *journeyClock, args ...string) *runningWorker {
	t.Helper()
	cfg, err := readWorkerFlags(args)
	if err != nil {
		t.Fatalf("read the worker's flags %v: %v", args, err)
	}
	shipped, err := known.Load()
	if err != nil {
		t.Fatalf("load the known servers: %v", err)
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	ticks := make(chan time.Time)
	parts := &workerParts{
		catalog: catalog, known: shipped, health: healthOf(j.pool),
		curator:      admin.NewCurator(j.pool).WithClock(clock.read),
		integrations: j.integrations, configPool: j.pool, toolTicks: ticks,
	}
	ctx, cancel := context.WithCancel(t.Context())
	stop := func() {
		cancel()
		_ = catalog.Close()
	}
	t.Cleanup(stop)
	if err := parts.connectTools(ctx, cfg, nil); err != nil {
		t.Fatalf("start the worker %v: %v", args, err)
	}
	return &runningWorker{ticks: ticks, stop: stop, catalog: catalog}
}

/*
The flag still names the server, but its process died and cannot be started
again. The worker no longer serves it, so it must stop renewing the lease:
once the lease runs out the tool leaves, as if the flag were gone. A worker
that renewed by the flag alone kept the dead server's tools for as long as it
ran.
*/
func TestCatalogue_aCommandLineServerThatDiedForGood_leavesTheCatalogue(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	clock := &journeyClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	tombstone := t.TempDir() + "/local-reports.dead"
	a := j.startWorker(t, clock, "--owner", "worker-a", "--mcp", localReportsFlag(tombstone))
	b := j.startWorker(t, clock, "--owner", "worker-b")
	if _, listed := j.listed(t)[localRunReport]; !listed {
		t.Fatalf("%s was never published; nothing below would prove anything", localRunReport)
	}

	a.kill(t)
	clock.advance(admin.ToolServerLease / 2)
	a.pass()
	clock.advance(admin.ToolServerLease/2 + time.Second)
	a.pass()
	b.pass()

	if _, still := j.listed(t)[localRunReport]; still {
		t.Fatalf("%s is still offered, though the only worker naming it holds a dead session", localRunReport)
	}
}

// A process that died but starts again is reconnected, and its worker goes
// on serving — and leasing — it.
func TestCatalogue_aCommandLineServerThatDiedAndRestarts_staysOffered(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	clock := &journeyClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	a := j.startWorker(t, clock, "--owner", "worker-a", "--mcp", localReportsFlag())
	b := j.startWorker(t, clock, "--owner", "worker-b")

	a.kill(t)
	clock.advance(admin.ToolServerLease / 2)
	a.pass()
	clock.advance(admin.ToolServerLease/2 + time.Second)
	a.pass()
	b.pass()

	if _, kept := j.listed(t)[localRunReport]; !kept || !a.catalog.Connected("local-reports") {
		t.Fatalf("%s listed %v, session connected %v; want it reconnected and still offered",
			localRunReport, kept, a.catalog.Connected("local-reports"))
	}
}

const localRunReport = "local-reports.run_report"

// classifyLocal rules run_report a write through the console's route.
func (j *factoryJourney) classifyLocal(t *testing.T) {
	t.Helper()
	tool, listed := j.listed(t)[localRunReport]
	if !listed {
		t.Fatalf("%s was never published; nothing below would prove anything", localRunReport)
	}
	got, err := j.api.ClassifyTool(installer(t), openapi.ClassifyToolRequestObject{
		ToolId: localRunReport,
		Body:   &openapi.ClassifyToolJSONRequestBody{Effect: openapi.EffectWrite, Digest: tool.Digest},
	})
	if _, ok := got.(openapi.ClassifyTool204Response); err != nil || !ok {
		t.Fatalf("classify %s = %#v, %v", localRunReport, got, err)
	}
}

/*
Two workers run local-reports from their command line. One restarts without
the flag: the other still serves the server, so its tools stay. The other
restarts without it too, and once its lease runs out the tools leave. The
ruling on them does not: the flag put back on a worker brings the tool back
with the decision already made.
*/
func TestCatalogue_aCommandLineServerDroppedFromEveryWorker_leavesTheCatalogue(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	clock := &journeyClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	a := j.startWorker(t, clock, "--owner", "worker-a", "--mcp", localReportsFlag())
	b := j.startWorker(t, clock, "--owner", "worker-b", "--mcp", localReportsFlag())
	j.classifyLocal(t)

	a.stop()
	a = j.startWorker(t, clock, "--owner", "worker-a")
	clock.advance(admin.ToolServerLease / 2)
	b.pass()
	clock.advance(admin.ToolServerLease/2 + time.Second)
	a.pass()
	b.pass()
	// A worker starting now publishes, and a publication sweeps whatever
	// nobody holds: worker-a's lease has run out, worker-b's must not have.
	j.startWorker(t, clock, "--owner", "worker-d")
	if _, kept := j.listed(t)[localRunReport]; !kept {
		t.Fatalf("%s left the catalogue while worker-b still serves it", localRunReport)
	}

	b.stop()
	b = j.startWorker(t, clock, "--owner", "worker-b")
	clock.advance(admin.ToolServerLease + time.Second)
	b.pass()
	if _, still := j.listed(t)[localRunReport]; still {
		t.Fatalf("%s is still offered after every worker dropped the flag", localRunReport)
	}

	j.startWorker(t, clock, "--owner", "worker-c", "--mcp", localReportsFlag())
	tool, back := j.listed(t)[localRunReport]
	if !back || tool.Effect != openapi.ToolEffect(domain.EffectWrite.String()) {
		t.Fatalf("%s reads %+v (listed %v); want it back with the write ruling it had", localRunReport, tool, back)
	}
}

/*
A rolling upgrade. A worker from before leases still runs local-reports from
its flag; it published the tool marked unmanaged, and never renews. A new
worker without the flag starts and passes: the tool stays for the grace,
because the old worker may still be serving it. When the grace is over and
nobody has renewed, it leaves.
*/
func TestCatalogue_anOldWorkersCommandLineServer_survivesTheRolloutGrace(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	clock := &journeyClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	if _, err := j.pool.Exec(t.Context(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_at)
		values ('installation', '', '', 'tool', $1,
		        '{"server": "local-reports", "onSurface": false, "unmanaged": true}', true, now())`,
		localRunReport); err != nil {
		t.Fatalf("publish as the old worker did: %v", err)
	}

	fresh := j.startWorker(t, clock, "--owner", "worker-new")
	clock.advance(admin.LegacyToolGrace - time.Second)
	fresh.pass()
	_, kept := j.listed(t)[localRunReport]
	clock.advance(2 * time.Second)
	fresh.pass()
	_, still := j.listed(t)[localRunReport]

	if !kept || still {
		t.Fatalf("%s listed inside the grace: %v, after it: %v; want kept, then gone",
			localRunReport, kept, still)
	}
}

// A worker that keeps the flag keeps its server offered, however long it
// runs: the lease its start-up took is renewed on every pass.
func TestCatalogue_aCommandLineServerStillRunning_staysPastTheFirstLease(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	clock := &journeyClock{now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	a := j.startWorker(t, clock, "--owner", "worker-a", "--mcp", localReportsFlag())
	b := j.startWorker(t, clock, "--owner", "worker-b")

	for range 4 {
		clock.advance(admin.ToolServerLease / 2)
		a.pass()
		b.pass()
	}
	// A publication sweeps whatever nobody holds, whether or not a pass did.
	j.startWorker(t, clock, "--owner", "worker-c")

	if _, kept := j.listed(t)[localRunReport]; !kept {
		t.Fatalf("%s left the catalogue though worker-a still runs it", localRunReport)
	}
}
