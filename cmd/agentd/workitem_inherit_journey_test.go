package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/vault"
)

/*
A run inherits a work item from how it was opened (#204, slice 3), journeyed
from the real entries: the /hooks door serve mounts, the dispatcher the worker
runs, the Postgres ledger openStore returns, and the trail route serve mounts.
*/

// inheritRig is one installation: its ledger, its configuration database and
// the authoring route that publishes agents into it.
type inheritRig struct {
	store Store
	pool  *pgxpool.Pool
	api   http.Handler
	run   string
}

func newInheritRig(t *testing.T, hooks trigger.Webhooks) inheritRig {
	t.Helper()
	store, err := openStore(context.Background(), testdb.Required(t))
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	pool := testdb.Pool(t)
	api := httpapi.NewServer(store, "test").
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{}))
	if hooks != nil {
		api = api.WithWebhooks(hooks)
	}
	return inheritRig{store: store, pool: pool, api: apiHandlerFor(api), run: fmt.Sprintf("i204-%d", time.Now().UnixNano())}
}

// publish puts an agent in acme/factory that may run unattended.
func (r inheritRig) publish(t *testing.T, def openapi.AgentDefinition) {
	t.Helper()
	def.Company, def.Area, def.Provider, def.Model = "acme", "factory", "openai", "gpt-test"
	def.Instructions, def.Tools = "Do the work.", &[]string{"crm.lookup"}
	def.Budget = &openapi.Budget{Micros: ptrTo(int64(500_000)), Steps: ptrTo(int64(60))}
	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/api/v1/agents/"+def.Name+"/versions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), openPrincipal()))
	rec := httptest.NewRecorder()
	r.api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT %s = %d: %s", def.Name, rec.Code, rec.Body)
	}
	state := spec.NewState(r.pool)
	agent := domain.AgentID(def.Name)
	if err := state.SetStage(t.Context(), agent, domain.StageCopilot, "usr_ana"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := state.SetPaused(t.Context(), agent, false, "usr_ana", time.Now()); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

// claimAndFinish runs agent's published version through a start, a claim of
// card and a finish, through the ledger's own append.
func (r inheritRig) claimAndFinish(t *testing.T, id domain.RunID, agent domain.AgentID, card string) {
	t.Helper()
	versions, err := spec.NewRegistry(r.pool).Versions(t.Context(), agent)
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions of %s: %v, %v", agent, versions, err)
	}
	claimed, err := json.Marshal(domain.WorkItemClaimedPayload{Ref: domain.WorkItemRef(card)})
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []domain.Step{
		{Kind: domain.StepRunStarted, Payload: []byte(`{"trigger":"manual"}`)},
		{Kind: domain.StepWorkItemClaimed, Payload: claimed},
		{Kind: domain.StepRunFinished, Payload: []byte(`{"reason":"finish_tool"}`)},
	} {
		st.RunID, st.AgentID, st.VersionID, st.At = id, agent, versions[0].VersionID, time.Now().UTC()
		st.Scope = domain.Scope{Company: "acme", Area: "factory"}
		if _, err := r.store.Append(t.Context(), st); err != nil {
			t.Fatalf("append %s: %v", st.Kind, err)
		}
	}
}

/*
onlyFinished is the Postgres store as the dispatcher's feed, with the feed's
position narrowed to one agent's finished run.

The real feed reads below the cluster's oldest open transaction and from one
cursor the whole database shares, so on a server other suites are using it
can hold this run back, or another test's dispatcher can move past it. What
the journey proves is downstream of that: the batch read (StartsAndFinishes,
the store's own), the precedence, the opener and the projection.
*/
type onlyFinished struct {
	Store
	run string
}

func (f onlyFinished) FinishedSince(ctx context.Context, after domain.FinishCursor, _ int) ([]domain.FinishedRun, error) {
	if after.Run != "" {
		return nil, nil
	}
	runs, err := f.ListRuns(ctx, domain.RunFilter{AgentID: domain.AgentID(f.run)}, "", 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return []domain.FinishedRun{{Run: runs[0], At: domain.FinishCursor{Xid: 1, Run: runs[0].RunID}}}, nil
}

func (onlyFinished) EventCursor(context.Context) (domain.FinishCursor, error) {
	return domain.FinishCursor{}, nil
}

func (onlyFinished) AdvanceEventCursor(context.Context, domain.FinishCursor) error { return nil }

// trail reads ref's trail through the route serve mounts, as a factory
// approver.
func (r inheritRig) trail(t *testing.T, ref string) openapi.WorkItemTrail {
	t.Helper()
	code, body := getTrail(t, apiHandlerFor(ledgerAPI(r.store, "test")), factoryApprover(), url.Values{"ref": {ref}})
	if code != http.StatusOK {
		t.Fatalf("GET trail = %d: %s", code, body)
	}
	var trail openapi.WorkItemTrail
	if err := json.Unmarshal(body, &trail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return trail
}

func TestJourney_aSignedGitHubIssuesOpened_putsTheRunOnTheIssuesTrail(t *testing.T) {
	v, err := vault.New(bytes.Repeat([]byte{7}, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	pool := testdb.Pool(t)
	hooks := webhooksWith(pool, v)
	rig := newInheritRig(t, hooks)
	agent, path := rig.run+"-triage", "gh/"+rig.run
	rig.publish(t, openapi.AgentDefinition{Name: agent, Triggers: &[]openapi.AgentTrigger{{
		Type: openapi.AgentTriggerTypeWebhook, Path: &path,
		From: ptrTo(openapi.AgentTriggerFromGithub), Events: &[]string{"issues.opened"},
	}}})
	secret, err := hooks.Rotate(t.Context(), trigger.Rotation{Path: path, Agent: domain.AgentID(agent), By: "usr_ana", At: time.Now()})
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	repo := "Journey-" + rig.run
	body := `{"action":"opened","repository":{"full_name":"FuseOne-IO/` + repo + `"},` +
		`"issue":{"number":77,"title":"github:evil/elsewhere#1"}}`
	mux := http.NewServeMux()
	webhookDoor(pool, rig.store, v).Mount(mux)
	req := httptest.NewRequest(http.MethodPost, "/hooks/"+path, strings.NewReader(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "delivery-"+rig.run)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("delivery = %d: %s", rec.Code, rec.Body)
	}

	issue := "github:fuseone-io/" + strings.ToLower(repo) + "#77"
	trail := rig.trail(t, issue)
	if len(trail.Runs) != 1 || trail.Runs[0].AgentId != agent || trail.Runs[0].Link != "github_delivery" {
		t.Fatalf("trail of %s = %+v; want the delivery's run", issue, trail.Runs)
	}
	if roles := *trail.Roles; len(roles) != 1 || roles[0].Runs != 1 {
		t.Fatalf("roles = %+v; the inherited run is counted", roles)
	}
}

/*
A Developer run claims a card and finishes; the event it publishes opens a
Reviewer. Swept by the dispatcher the worker builds, over the Postgres feed,
the Reviewer is on the Developer's card — and its own agent may still claim
the card, because the inheritance holds nothing.
*/
func TestJourney_anEventOpenedListenerOfADeveloperRun_showsUnderTheDevelopersCard(t *testing.T) {
	rig := newInheritRig(t, nil)
	developer, reviewer := rig.run+"-developer", rig.run+"-reviewer"
	ready := rig.run + ".code.ready"
	rig.publish(t, openapi.AgentDefinition{Name: developer, Emits: &[]openapi.AgentEvent{{Event: ready}}})
	rig.publish(t, openapi.AgentDefinition{Name: reviewer,
		Triggers: &[]openapi.AgentTrigger{{Type: "event", Event: &ready}}})
	card := fmt.Sprintf("github:fuseone-io/journey-%s#204", rig.run)
	rig.claimAndFinish(t, domain.RunID("run-"+developer), domain.AgentID(developer), card)

	parts := &workerParts{store: rig.store, registry: spec.NewRegistry(rig.pool), configPool: rig.pool}
	dispatcher := trigger.NewDispatcher(parts.registry, onlyFinished{Store: parts.store, run: developer},
		parts.opener(), engine.SystemClock{}, nil)
	if _, err := dispatcher.Sweep(t.Context(), trigger.Page); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	trail := rig.trail(t, card)
	var listener *openapi.WorkItemRun
	for i, r := range trail.Runs {
		if r.AgentId == reviewer {
			listener = &trail.Runs[i]
		}
	}
	if listener == nil || listener.Link != "event" || listener.Ref != card {
		t.Fatalf("trail of %s = %+v; want the reviewer by the event", card, trail.Runs)
	}
	raw, err := json.Marshal(domain.WorkItemClaimedPayload{Ref: domain.WorkItemRef(card)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rig.store.Append(t.Context(), domain.Step{RunID: domain.RunID(listener.RunId),
		Kind: domain.StepWorkItemClaimed, AgentID: domain.AgentID(reviewer),
		Scope: domain.Scope{Company: "acme", Area: "factory"}, Payload: raw}); err != nil {
		t.Fatalf("the reviewer's own claim of the card it inherited = %v, want success", err)
	}
}
