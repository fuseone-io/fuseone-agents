package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Activating a candidate, from the route to the version every reader serves (NT-018 phase 2,
release 2c).

The agent is published through the publication route, the candidate sealed the way the
seal route writes it, and activated through the activation route with the agent's tag.
The sealed bytes become the current version, its cron trigger reaches the schedules, and
an erasure of the seal is waited for, or waits, at the agent's content fence: first, the
activation finds the tombstone and changes nothing; second, the version it made live keeps
its text, as every published version does.
*/

const (
	activationSeal = "9e5f6a7b-8c9d-4e0f-8a1b-2c3d4e5f6a7b"
	activationCron = "*/20 * * * *"
)

const activationTemplate = `---
id: %s
name: Triage
company: acme
area: cx
provider: primary
model: declared-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 20
triggers:
  - { type: cron, schedule: "` + activationCron + `" }
---

Classify the ticket from the activated text.

%s
`

// activationBytes is a candidate of agent, with never as its never block ("" for none).
func activationBytes(agent domain.AgentID, never string) string {
	block := ""
	if never != "" {
		block = "Never\n" + never
	}
	return fmt.Sprintf(activationTemplate, agent, block)
}

// activationWorld is the experiment world with activation wired as serve wires it, its
// reconciliation included.
func activationWorld(t *testing.T) *experimentWorld {
	t.Helper()
	w := newExperimentWorld(t)
	content := ledger.NewContent(w.pool)
	w.server.WithActivations(spec.NewActivations(spec.NewPublisher(w.pool, engine.SystemClock{}),
		spec.NewCandidateReader(w.pool, content)).
		WithReconciliation(trigger.NewPostgresSchedules(w.pool), trigger.NewPostgresWebhooks(w.pool)))
	return w
}

// sealText seals text for agent under sealID, as the seal route writes a seal.
func (w *experimentWorld) sealText(t *testing.T, sealID, text string) domain.VersionID {
	t.Helper()
	sealed, err := spec.Parse("triage.agent.md", []byte(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sum := sha256.Sum256([]byte(text))
	if _, err := w.pool.Exec(t.Context(), `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)
		on conflict do nothing`, string(sealed.ID), string(sealed.Version), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	ref, err := ledger.NewContent(w.pool).PutFor(t.Context(), "authoring", sealID, 0, []byte(text))
	if err != nil {
		t.Fatalf("keep the sealed bytes: %v", err)
	}
	if _, err := w.pool.Exec(t.Context(), `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id,
		  content_ref, draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, 'acme', 'cx', $4, gen_random_uuid(), 'rev-1', 'usr_ana')`,
		sealID, string(sealed.ID), string(sealed.Version), ref); err != nil {
		t.Fatalf("seal: %v", err)
	}
	return sealed.Version
}

// currentOf is the version every reader serves for agent.
func (w *experimentWorld) currentOf(t *testing.T, agent domain.AgentID) domain.VersionID {
	t.Helper()
	var current *string
	if err := w.pool.QueryRow(t.Context(),
		`select current_version from agent_state where agent_id = $1`, string(agent)).Scan(&current); err != nil {
		t.Fatalf("read the current version: %v", err)
	}
	if current == nil {
		versions, err := spec.NewRegistry(w.pool).Versions(t.Context(), agent)
		if err != nil || len(versions) == 0 {
			t.Fatalf("Versions = %v, %v", versions, err)
		}
		return versions[0].VersionID
	}
	return domain.VersionID(*current)
}

func (w *experimentWorld) activate(
	t *testing.T, ctx context.Context, agent domain.AgentID, version domain.VersionID, ifMatch, ifNoneMatch *string,
) openapi.ActivateAgentVersionResponseObject {
	t.Helper()
	resp, err := w.server.ActivateAgentVersion(ctx, openapi.ActivateAgentVersionRequestObject{
		AgentId: string(agent), Version: string(version),
		Params: openapi.ActivateAgentVersionParams{IfMatch: ifMatch, IfNoneMatch: ifNoneMatch},
	})
	if err != nil {
		t.Fatalf("ActivateAgentVersion: %v", err)
	}
	return resp
}

func tagOf(version domain.VersionID) *string {
	tag := string(domain.AgentRevision(version, false))
	return &tag
}

func (w *experimentWorld) schedulesOf(t *testing.T, agent domain.AgentID) []string {
	t.Helper()
	rows, err := w.pool.Query(t.Context(),
		`select schedule from trigger_schedules where agent_id = $1 order by schedule`, string(agent))
	if err != nil {
		t.Fatalf("read schedules: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func activationConflict(resp openapi.ActivateAgentVersionResponseObject) string {
	if c, ok := resp.(openapi.ActivateAgentVersion409ApplicationProblemPlusJSONResponse); ok && c.Type != nil {
		return *c.Type
	}
	return fmt.Sprintf("%T", resp)
}

func TestActivation_fromTheRoute_theSealedBytesGoLiveAndTheirScheduleIsReconciled(t *testing.T) {
	w := activationWorld(t)
	live := w.currentOf(t, w.agent)
	text := activationBytes(w.agent, "Refund a ticket.")
	version := w.sealText(t, activationSeal, text)

	resp := w.activate(t, author(t.Context()), w.agent, version, tagOf(live), nil)
	ok, isOK := resp.(openapi.ActivateAgentVersion200JSONResponse)
	if !isOK || !ok.Body.Created || ok.Headers.ETag != *tagOf(version) {
		t.Fatalf("activate answered %T %+v, want the candidate created with its tag", resp, resp)
	}
	if got := w.currentOf(t, w.agent); got != version {
		t.Fatalf("current = %s, want the activated %s", got, version)
	}
	instructions, _, err := spec.NewRegistry(w.pool).Instructions(t.Context(), w.agent, version)
	if err != nil || !strings.Contains(instructions, "from the activated text") {
		t.Errorf("instructions = %q (%v), want the sealed text", instructions, err)
	}
	if got := w.schedulesOf(t, w.agent); len(got) != 1 || got[0] != activationCron {
		t.Errorf("schedules = %v, want the activated version's cron", got)
	}
}

func TestActivation_fromTheRoute_aCandidateWithNoNeverBlock_changesNothing(t *testing.T) {
	w := activationWorld(t)
	live := w.currentOf(t, w.agent)
	version := w.sealText(t, activationSeal, activationBytes(w.agent, ""))

	resp := w.activate(t, author(t.Context()), w.agent, version, tagOf(live), nil)
	bad, ok := resp.(openapi.ActivateAgentVersion400ApplicationProblemPlusJSONResponse)
	if !ok || bad.Type == nil || *bad.Type != "fuseone:publish-needs-never" {
		t.Fatalf("activate answered %T %+v, want 400 publish-needs-never", resp, resp)
	}
	if got := w.currentOf(t, w.agent); got != live {
		t.Errorf("current = %s, want %s still live", got, live)
	}
}

func TestActivation_fromTheRoute_aFirstVersionGoesLivePausedInTheSealedScope(t *testing.T) {
	w := activationWorld(t)
	fresh := domain.AgentID("fresh-triage")
	version := w.sealText(t, activationSeal, activationBytes(fresh, "Refund a ticket."))

	resp := w.activate(t, author(t.Context()), fresh, version, nil, ptrTo("*"))
	ok, isOK := resp.(openapi.ActivateAgentVersion200JSONResponse)
	if !isOK || !ok.Body.Paused || w.currentOf(t, fresh) != version {
		t.Fatalf("activate answered %T %+v, want a first version live and paused", resp, resp)
	}
}

func TestActivation_fromTheRoute_aRetryWithTheNewTagSucceedsAndCreatesNothing(t *testing.T) {
	w := activationWorld(t)
	live := w.currentOf(t, w.agent)
	version := w.sealText(t, activationSeal, activationBytes(w.agent, "Refund a ticket."))
	if _, ok := w.activate(t, author(t.Context()), w.agent, version, tagOf(live), nil).(openapi.ActivateAgentVersion200JSONResponse); !ok {
		t.Fatal("the first activation was refused")
	}

	again, ok := w.activate(t, author(t.Context()), w.agent, version, tagOf(version), nil).(openapi.ActivateAgentVersion200JSONResponse)
	if !ok || again.Body.Created || again.Body.SealId != nil {
		t.Errorf("a retry = %+v, want it live again from its row, nothing created", again)
	}
}

func TestActivation_anErasureHoldingTheFence_isWaitedFor_andTheTombstoneRefused(t *testing.T) {
	w := activationWorld(t)
	live := w.currentOf(t, w.agent)
	version := w.sealText(t, activationSeal, activationBytes(w.agent, "Refund a ticket."))
	ctx := context.Background()
	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		erased <- eraseCandidate(ctx, w.pool, regression.NewStore(w.pool), w, activationSeal, entered, release)
	}()
	<-entered

	done := make(chan openapi.ActivateAgentVersionResponseObject, 1)
	go func() { done <- w.activate(t, author(ctx), w.agent, version, tagOf(live), nil) }()
	select {
	case resp := <-done:
		t.Fatalf("the activation did not wait for the erasure's fence: %T", resp)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	if err := <-erased; err != nil {
		t.Fatalf("erase: %v", err)
	}

	if code := activationConflict(<-done); code != "fuseone:candidate-unreadable" {
		t.Fatalf("after the erasure = %s, want fuseone:candidate-unreadable", code)
	}
	if got := w.currentOf(t, w.agent); got != live {
		t.Errorf("current = %s, want %s still live", got, live)
	}
}

// stallingHold holds the activation inside its fence until released.
type stallingHold struct {
	*regression.Store
	entered, release chan struct{}
}

func (f stallingHold) Hold(ctx context.Context, agent domain.AgentID, inside func(context.Context) error) error {
	return f.Store.Hold(ctx, agent, func(ctx context.Context) error {
		close(f.entered)
		<-f.release
		return inside(ctx)
	})
}

func TestActivation_holdingTheFence_isWaitedForByTheErasure_andKeepsItsText(t *testing.T) {
	w := activationWorld(t)
	live := w.currentOf(t, w.agent)
	version := w.sealText(t, activationSeal, activationBytes(w.agent, "Refund a ticket."))
	ctx := context.Background()
	fence := stallingHold{Store: regression.NewStore(w.pool), entered: make(chan struct{}), release: make(chan struct{})}
	w.server.WithRegressions(fence)

	done := make(chan openapi.ActivateAgentVersionResponseObject, 1)
	go func() { done <- w.activate(t, author(ctx), w.agent, version, tagOf(live), nil) }()
	<-fence.entered
	eraseEntered, eraseRelease, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	close(eraseRelease)
	go func() {
		erased <- eraseCandidate(ctx, w.pool, regression.NewStore(w.pool), w, activationSeal, eraseEntered, eraseRelease)
	}()
	select {
	case <-eraseEntered:
		t.Fatal("the erasure entered the fence the activation holds")
	case <-time.After(400 * time.Millisecond):
	}
	close(fence.release)

	if resp, ok := (<-done).(openapi.ActivateAgentVersion200JSONResponse); !ok {
		t.Fatalf("the activation that held the fence = %T %+v, want it live", resp, resp)
	}
	if err := <-erased; err != nil {
		t.Fatalf("erase: %v", err)
	}
	instructions, _, err := spec.NewRegistry(w.pool).Instructions(ctx, w.agent, version)
	if err != nil || !strings.Contains(instructions, "from the activated text") || w.currentOf(t, w.agent) != version {
		t.Errorf("after the erasure: instructions %q (%v), current %s; want the live version's text kept", instructions, err, w.currentOf(t, w.agent))
	}
}

func TestActivation_fromTheRoute_aWebhookPathAnotherAgentHolds_changesNothing(t *testing.T) {
	w := activationWorld(t)
	live := w.currentOf(t, w.agent)
	text := strings.Replace(activationBytes(w.agent, "Refund a ticket."),
		"triggers:", "triggers:\n  - { type: webhook, path: crm/ticket }", 1)
	version := w.sealText(t, activationSeal, text)
	if err := trigger.NewPostgresWebhooks(w.pool).Sync(t.Context(), "someone-else",
		domain.Scope{Company: "acme", Area: "cx"}, []domain.WebhookTrigger{{Path: "crm/ticket"}}); err != nil {
		t.Fatalf("take the path: %v", err)
	}

	if code := activationConflict(w.activate(t, author(t.Context()), w.agent, version, tagOf(live), nil)); code != "fuseone:webhook-path-taken" {
		t.Fatalf("activate = %s, want fuseone:webhook-path-taken", code)
	}
	if got := w.currentOf(t, w.agent); got != live {
		t.Errorf("current = %s, want %s still live", got, live)
	}
	if got := w.schedulesOf(t, w.agent); len(got) != 0 {
		t.Errorf("schedules = %v, want none from an activation that was undone", got)
	}
}

// stallingSchedules holds the first reconciliation until released.
type stallingSchedules struct {
	*trigger.PostgresSchedules
	once             *sync.Once
	entered, release chan struct{}
}

func (s stallingSchedules) SyncIn(
	ctx context.Context, tx pgx.Tx, agent domain.AgentID, schedules []string, from time.Time,
) error {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.PostgresSchedules.SyncIn(ctx, tx, agent, schedules, from)
}

/*
A slower activation never puts its triggers back after a later one. A stalls while it
reconciles; B, made against the tag A will leave, waits for A's row lock instead of
committing around it, and reconciles after A. The schedules end as B declares them.
*/
func TestActivation_aStalledReconciliation_holdsTheNextActivation_andTheLatestWins(t *testing.T) {
	w := newExperimentWorld(t)
	stall := stallingSchedules{PostgresSchedules: trigger.NewPostgresSchedules(w.pool), once: &sync.Once{},
		entered: make(chan struct{}), release: make(chan struct{})}
	w.server.WithActivations(spec.NewActivations(spec.NewPublisher(w.pool, engine.SystemClock{}),
		spec.NewCandidateReader(w.pool, ledger.NewContent(w.pool))).
		WithReconciliation(stall, trigger.NewPostgresWebhooks(w.pool)))
	live := w.currentOf(t, w.agent)
	a := w.sealText(t, activationSeal, activationBytes(w.agent, "Refund a ticket."))
	b := w.sealText(t, "0a1b2c3d-4e5f-4a6b-8c7d-8e9f0a1b2c3d", strings.Replace(
		activationBytes(w.agent, "Refund a ticket."), activationCron, "*/30 * * * *", 1))
	ctx := context.Background()

	first := make(chan openapi.ActivateAgentVersionResponseObject, 1)
	go func() { first <- w.activate(t, author(ctx), w.agent, a, tagOf(live), nil) }()
	<-stall.entered
	second := make(chan openapi.ActivateAgentVersionResponseObject, 1)
	go func() { second <- w.activate(t, author(ctx), w.agent, b, tagOf(a), nil) }()
	select {
	case resp := <-second:
		t.Fatalf("B committed around A's reconciliation: %T %+v", resp, resp)
	case <-time.After(400 * time.Millisecond):
	}
	close(stall.release)

	for name, done := range map[string]chan openapi.ActivateAgentVersionResponseObject{"A": first, "B": second} {
		if resp, ok := (<-done).(openapi.ActivateAgentVersion200JSONResponse); !ok {
			t.Fatalf("%s = %T %+v, want it live", name, resp, resp)
		}
	}
	if got := w.schedulesOf(t, w.agent); len(got) != 1 || got[0] != "*/30 * * * *" {
		t.Errorf("schedules = %v, want B's alone", got)
	}
}
