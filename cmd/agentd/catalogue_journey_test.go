package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

/*
A worker that read the configuration just before the removal still held
the server, and published its tools just after: the rows the removal
dropped are back. Its next pass no longer wants the server and says so, and
that withdrawal is what takes them off the screen for good — the worker that
brought them back is the one that clears them.
*/
func TestCatalogue_toolsPublishedByAWorkerThatMissedTheRemoval_goOnItsNextPass(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	j.remove(t)
	if err := j.curator.Publish(t.Context(), j.catalog.Publication()); err != nil {
		t.Fatalf("the late publish: %v", err)
	}
	if len(ofServer(j.listed(t), "github-triage")) == 0 {
		t.Fatal("the late publish did not bring the tools back; the pass below would prove nothing")
	}

	j.worker.reconcile(t.Context())

	if left := ofServer(j.listed(t), "github-triage"); len(left) != 0 {
		t.Fatalf("the deleted server still offers %v after the worker's next pass", left)
	}
}

// A withdrawal is judged when it is recorded, not when it was decided: a
// server registered again in between is wanted, and keeps its tools.
func TestCatalogue_aWithdrawalOfAServerThatIsConfiguredAgain_keepsItsTools(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)

	if err := j.curator.Publish(t.Context(), domain.ToolPublication{Withdrawn: []string{"github-triage"}}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if len(ofServer(j.listed(t), "github-triage")) == 0 {
		t.Fatal("a stale withdrawal took the tools of a configured, enabled server")
	}
}

// Switched off, a server offers nothing to any run, and the catalogue says
// so; switched on again, discovery brings its tools back with their rulings.
func TestCatalogue_aDisabledServer_stopsOfferingItsTools(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	resp, err := j.api.PutMCPServer(installer(t), openapi.PutMCPServerRequestObject{
		Name: "github-triage", Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptrTo(openapi.TransportHttp), Url: &j.remote, Enabled: ptrTo(false),
		},
	})
	if _, ok := resp.(openapi.PutMCPServer204Response); err != nil || !ok {
		detail := ""
		if refused, was := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse); was && refused.Detail != nil {
			detail = *refused.Detail
		}
		t.Fatalf("disable github-triage = %#v (%s), %v; want 204", resp, detail, err)
	}

	j.worker.reconcile(t.Context())

	if left := ofServer(j.listed(t), "github-triage"); len(left) != 0 {
		t.Fatalf("a disabled server still offers %v", left)
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
