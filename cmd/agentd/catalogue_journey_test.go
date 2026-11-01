package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/tools"
)

/*
The published catalogue follows what the servers offer (#219).

It used to only grow. A discovery recorded what it found and never removed
what it no longer saw, so a registration narrowed after a broader discovery
kept offering — on the console and to the publish check — tools the runtime
could no longer call, and a deleted server's tools stayed forever.

These start where the operator does: the registration route, a worker's
reconcile pass, and the console's tool list and publish route.
*/

// listed is the console's tool list, by ID.
func (j *factoryJourney) listed(t *testing.T) map[string]openapi.Tool {
	t.Helper()
	resp, err := j.api.ListTools(installer(t), openapi.ListToolsRequestObject{})
	page, ok := resp.(openapi.ListTools200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("ListTools = %#v, %v", resp, err)
	}
	out := make(map[string]openapi.Tool, len(page.Items))
	for _, tool := range page.Items {
		out[tool.ToolId] = tool
	}
	return out
}

var projectTools = []string{"projects_list", "projects_get", "projects_write"}

// narrow saves the registration again with other toolsets, as an operator
// correcting the header would, and lets the worker rediscover.
func (j *factoryJourney) narrow(t *testing.T, toolsets string) {
	t.Helper()
	// The fingerprint a reconnect is judged by carries the save's time; two
	// saves inside one clock tick would read as one.
	time.Sleep(10 * time.Millisecond)
	j.register(t, j.remote, toolsets)
	j.discover(t)
}

func TestCatalogue_toolsetsNarrowedAfterADiscovery_stopOfferingTheDroppedTools(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	j.classify(t)

	j.narrow(t, "issues")

	listed := j.listed(t)
	for _, name := range projectTools {
		if _, still := listed["github-triage."+name]; still {
			t.Errorf("github-triage.%s is still offered after the toolsets dropped projects", name)
		}
	}
	if _, kept := listed["github-triage.issue_read"]; !kept {
		t.Error("github-triage.issue_read disappeared, though the narrowed toolsets still offer it")
	}
	refused, ok := j.publish(t, readTriageReference(t)).(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Detail == nil || !strings.Contains(*refused.Detail, "github-triage.projects_write") {
		t.Fatalf("publishing the Triage reference answered %#v; want it refused for the dropped tools", refused)
	}
}

func TestCatalogue_toolsetsWidenedAgain_bringBackTheRulingsTheToolsHad(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	j.classify(t)
	j.narrow(t, "issues")

	j.narrow(t, triageToolsets)

	tool, back := j.listed(t)["github-triage.projects_write"]
	if !back || tool.Effect != openapi.ToolEffect(domain.EffectWrite.String()) {
		t.Fatalf("projects_write reads %+v (listed %v); want it back with the write ruling it had", tool, back)
	}
}

// remove deletes the registration through the console's route.
func (j *factoryJourney) remove(t *testing.T) {
	t.Helper()
	resp, err := j.api.DeleteMCPServer(installer(t), openapi.DeleteMCPServerRequestObject{Name: "github-triage"})
	if _, ok := resp.(openapi.DeleteMCPServer204Response); err != nil || !ok {
		t.Fatalf("DeleteMCPServer = %#v, %v; want 204", resp, err)
	}
}

// ofServer is the listed tools the server offered.
func ofServer(listed map[string]openapi.Tool, server string) []string {
	var out []string
	for id := range listed {
		if strings.HasPrefix(id, server+".") {
			out = append(out, id)
		}
	}
	return out
}

// Removed is removed on the screen at once, whether or not a worker is
// running to notice.
func TestCatalogue_aDeletedServer_stopsOfferingItsTools(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)

	j.remove(t)

	if left := ofServer(j.listed(t), "github-triage"); len(left) != 0 {
		t.Fatalf("a deleted server still offers %v", left)
	}
}

// Removing a server is not revoking the Curator's rulings on its tools. The
// same server registered again offers the tools somebody already ruled on.
func TestCatalogue_aServerRegisteredAgainAfterRemoval_findsItsRulings(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	j.classify(t)
	j.remove(t)
	j.worker.reconcile(t.Context())

	j.register(t, j.remote, triageToolsets)
	j.discover(t)

	tool, back := j.listed(t)["github-triage.projects_write"]
	if !back || tool.Effect != openapi.ToolEffect(domain.EffectWrite.String()) {
		t.Fatalf("projects_write reads %+v (listed %v); want the write ruling it had", tool, back)
	}
}

// disable switches the registration off through the console's route.
func (j *factoryJourney) disable(t *testing.T) {
	t.Helper()
	resp, err := j.api.PutMCPServer(installer(t), openapi.PutMCPServerRequestObject{
		Name: "github-triage", Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptrTo(openapi.TransportHttp), Url: &j.remote, Enabled: ptrTo(false),
		},
	})
	if _, ok := resp.(openapi.PutMCPServer204Response); err != nil || !ok {
		t.Fatalf("disable github-triage = %#v, %v; want 204", resp, err)
	}
}

// replica is a second worker over the same database: its own catalogue, the
// real connect path, publishing to the same Curator.
func (j *factoryJourney) replica(t *testing.T) *reconciler {
	t.Helper()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	return newReconciler(catalog, j.integrations, healthOf(j.pool)).
		publishingTo(j.curator).classifyingWith(j.curator)
}

// latePublish is a worker publishing what it holds without first reading the
// configuration again — the publication it would have sent from a pass that
// began before the change.
func latePublish(t *testing.T, worker *reconciler) {
	t.Helper()
	if err := worker.publisher.Publish(t.Context(), worker.publication()); err != nil {
		t.Fatalf("the late publish: %v", err)
	}
}

// refused reports whether publishing the Triage reference is refused for
// naming tools the installation does not offer.
func (j *factoryJourney) refused(t *testing.T) bool {
	t.Helper()
	got, ok := j.publish(t, readTriageReference(t)).(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	return ok && got.Detail != nil && strings.Contains(*got.Detail, "github-triage.")
}

/*
The review's sequence, replica for replica. A and B both hold the server. It
is deleted; B notices and publishes. A, whose pass began before the
deletion, publishes its discovery afterwards — and then stops for good. B
passes again with nothing left to notice.

A late publication used to put the tools back, and with A gone nothing ever
took them away again: the Triage reference published against a server that
did not exist. The administration now judges A's discovery against the
configuration it holds, under which the server is gone.
*/
func TestCatalogue_aLatePublicationFromAReplicaThatMissedTheRemoval_putsNothingBack(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	a := j.worker
	j.discover(t)
	j.classify(t)
	b := j.replica(t)
	b.reconcile(t.Context())

	j.remove(t)
	b.reconcile(t.Context())
	latePublish(t, a)
	b.reconcile(t.Context())

	if left := ofServer(j.listed(t), "github-triage"); len(left) != 0 {
		t.Fatalf("the deleted server offers %v after a late publication", left)
	}
	if !j.refused(t) {
		t.Fatal("the Triage reference published against a deleted server")
	}
}

// The same sequence with the server switched off instead of deleted.
func TestCatalogue_aLatePublicationFromAReplicaThatMissedTheDisable_putsNothingBack(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	a := j.worker
	j.discover(t)
	j.classify(t)
	b := j.replica(t)
	b.reconcile(t.Context())

	j.disable(t)
	b.reconcile(t.Context())
	latePublish(t, a)
	b.reconcile(t.Context())

	if left := ofServer(j.listed(t), "github-triage"); len(left) != 0 {
		t.Fatalf("the disabled server offers %v after a late publication", left)
	}
	if !j.refused(t) {
		t.Fatal("the Triage reference published against a disabled server")
	}
}

// A replica that discovered the broad toolsets publishes after another has
// recorded the narrowed ones: its discovery is of a configuration that is no
// longer in force, and the dropped tools stay dropped.
func TestCatalogue_aLateBroadDiscoveryAfterTheToolsetsNarrowed_isIgnored(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	a := j.worker
	j.discover(t)
	j.classify(t)
	b := j.replica(t)
	b.reconcile(t.Context())

	time.Sleep(10 * time.Millisecond)
	j.register(t, j.remote, "issues")
	b.reconcile(t.Context())
	latePublish(t, a)

	listed := j.listed(t)
	for _, name := range projectTools {
		if _, back := listed["github-triage."+name]; back {
			t.Errorf("github-triage.%s is back after a late discovery of the old toolsets", name)
		}
	}
	if _, kept := listed["github-triage.issue_read"]; !kept {
		t.Error("github-triage.issue_read is gone, though the current toolsets offer it")
	}
}

// Switched off, a server offers nothing to any run, and the catalogue says
// so at the next pass.
func TestCatalogue_aDisabledServer_stopsOfferingItsTools(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	j.disable(t)

	j.worker.reconcile(t.Context())

	if left := ofServer(j.listed(t), "github-triage"); len(left) != 0 {
		t.Fatalf("a disabled server still offers %v", left)
	}
}

/*
A server started from the command line (--mcp) is not in the configuration,
so nothing there can judge it: its own worker's discovery is the authority.
Another replica's publication sweeps away the tools of servers the
configuration no longer wants, and must leave this one alone.
*/
func TestCatalogue_aCommandLineServer_survivesAnotherReplicasPublication(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	local := &testSession{tools: []*mcp.Tool{{Name: "run_report"}}}
	if err := j.catalog.AddServer(t.Context(), "local-reports", local, nil); err != nil {
		t.Fatalf("connect the command-line server: %v", err)
	}
	j.worker.hold("local-reports")
	j.discover(t)
	if _, listed := j.listed(t)["local-reports.run_report"]; !listed {
		t.Fatal("the command-line server's tool was never published; the sweep below would prove nothing")
	}

	j.replica(t).reconcile(t.Context())

	if _, kept := j.listed(t)["local-reports.run_report"]; !kept {
		t.Fatal("another replica's publication swept the command-line server's tool away")
	}
}

// A second worker that cannot reach the server — not yet connected, or the
// server down from where it stands — publishes too, and must not take the
// tools another worker discovered off the screen.
func TestCatalogue_aWorkerThatCannotReachTheServer_leavesItsToolsPublished(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	second := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = second.Close() })
	unreachable := newReconciler(second, j.integrations, healthOf(j.pool)).
		publishingTo(j.curator).classifyingWith(j.curator)
	unreachable.connectTo = func(
		context.Context, *tools.Catalog, domain.MCPServer,
		domain.MCPCredentials, OAuthGrantStore, MCPUserCredentialStore, credentialPolicy,
		stdioEgressObserver,
	) error {
		return errors.New("dial tcp: connection refused")
	}

	unreachable.reconcile(t.Context())

	listed := j.listed(t)
	for _, name := range append([]string{"issue_read"}, projectTools...) {
		if _, kept := listed["github-triage."+name]; !kept {
			t.Errorf("github-triage.%s vanished after a worker that never reached the server published", name)
		}
	}
}
