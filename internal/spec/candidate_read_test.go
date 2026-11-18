package spec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
)

/*
Listing and reading candidates (NT-018 phase 2, release 2a).

Every seal is authorized on its own sealed scope, with agent:publish: a candidate is
unpublished work. A version that has a row in agent_specs is published, not a candidate,
whatever seals it has. Reading answers the newest visible seal whose bytes check out, and
says which seal it read; with every visible seal unreadable it is unreadable.
*/

const candidateText = `---
id: %s
name: Triage
company: acme
area: %s
provider: openai
model: test-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
---

Read the sealed ticket.
`

type candidateWorld struct {
	pool    *pgxpool.Pool
	content *ledger.Content
	reader  *spec.CandidateReader
	agent   domain.AgentID
}

func newCandidateWorld(t *testing.T) candidateWorld {
	t.Helper()
	pool := openSpecPool(t)
	content := ledger.NewContent(pool)
	return candidateWorld{
		pool: pool, content: content, reader: spec.NewCandidateReader(pool, content),
		agent: domain.AgentID("cand-" + time.Now().Format("150405.000000000")),
	}
}

// seal writes one seal of text in area, at, and returns its id and the version.
func (w candidateWorld) seal(t *testing.T, area string, at time.Time) (string, domain.VersionID) {
	t.Helper()
	ctx := context.Background()
	text := []byte(fmt.Sprintf(candidateText, w.agent, area))
	parsed, err := spec.Parse("c.agent.md", text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sum := sha256.Sum256(text)
	if _, err := w.pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)
		on conflict do nothing`, string(w.agent), string(parsed.Version), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	var id string
	if err := w.pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ref, err := w.content.PutFor(ctx, "authoring", id, 0, text)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by, sealed_at)
		values ($1::uuid, $2, $3, 'acme', $4, $5, gen_random_uuid(), 'rev', 'usr_ana', $6)`,
		id, string(w.agent), string(parsed.Version), area, ref, at); err != nil {
		t.Fatalf("seal: %v", err)
	}
	return id, parsed.Version
}

func (w candidateWorld) erase(t *testing.T, seal string) {
	t.Helper()
	tx, err := w.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.content.EraseOwnersTx(context.Background(), tx, "authoring", []string{seal}, "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func allowAreas(areas ...string) func(domain.Scope) bool {
	return func(s domain.Scope) bool {
		for _, a := range areas {
			if s == (domain.Scope{Company: "acme", Area: domain.AreaID(a)}) {
				return true
			}
		}
		return false
	}
}

func TestCandidates_listsEachVisibleSealNewestFirst_andNoPublishedVersion(t *testing.T) {
	w := newCandidateWorld(t)
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	older, version := w.seal(t, "cx", t0)
	newer, _ := w.seal(t, "cx", t0.Add(time.Minute))
	hidden, _ := w.seal(t, "finance", t0.Add(2*time.Minute))
	_, publishedVersion := w.seal(t, "sales", t0.Add(3*time.Minute))
	// Published by the ordinary path afterwards: from now on it is a version, not a
	// candidate, and its seal stays as history.
	sales, err := spec.Parse("c.agent.md", []byte(fmt.Sprintf(candidateText, w.agent, "sales")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := spec.NewRegistry(w.pool).Publish(context.Background(), sales, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got, err := w.reader.List(context.Background(), w.agent, allowAreas("cx", "sales"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var ids []string
	for _, c := range got {
		ids = append(ids, c.SealID)
		if c.Version == publishedVersion {
			t.Errorf("listed the published version %s", publishedVersion)
		}
		if c.SealID == hidden {
			t.Errorf("listed a seal outside the caller's scope")
		}
	}
	if len(ids) != 2 || ids[0] != newer || ids[1] != older || got[0].Version != version {
		t.Fatalf("listed %v, want [%s %s] of %s", ids, newer, older, version)
	}
	if _, err := w.reader.Read(context.Background(), w.agent, publishedVersion, allowAreas("sales")); !errors.Is(err, spec.ErrCandidateNotFound) {
		t.Errorf("reading the published version as a candidate: err = %v, want ErrCandidateNotFound", err)
	}
}

func TestCandidates_readsTheNewestVisibleReadableSeal(t *testing.T) {
	w := newCandidateWorld(t)
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	older, version := w.seal(t, "cx", t0)
	newest, _ := w.seal(t, "cx", t0.Add(time.Minute))
	w.erase(t, newest)

	got, err := w.reader.Read(context.Background(), w.agent, version, allowAreas("cx"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.SealID != older || got.Definition != fmt.Sprintf(candidateText, w.agent, "cx") {
		t.Fatalf("read seal %s (%d bytes), want the older readable seal %s", got.SealID, len(got.Definition), older)
	}
}

func TestCandidates_refusals(t *testing.T) {
	w := newCandidateWorld(t)
	t0 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	only, version := w.seal(t, "cx", t0)

	if _, err := w.reader.Read(context.Background(), w.agent, version, allowAreas("finance")); !errors.Is(err, spec.ErrCandidateNotFound) {
		t.Errorf("outside the sealed scope: err = %v, want ErrCandidateNotFound", err)
	}
	if _, err := w.reader.Read(context.Background(), w.agent, "v000000000000", allowAreas("cx")); !errors.Is(err, spec.ErrCandidateNotFound) {
		t.Errorf("a version nobody sealed: err = %v, want ErrCandidateNotFound", err)
	}
	w.erase(t, only)
	if _, err := w.reader.Read(context.Background(), w.agent, version, allowAreas("cx")); !errors.Is(err, domain.ErrCandidateUnreadable) {
		t.Errorf("every visible seal erased: err = %v, want ErrCandidateUnreadable", err)
	}
	if _, err := w.reader.Read(context.Background(), w.agent, version, allowAreas("finance")); !errors.Is(err, spec.ErrCandidateNotFound) {
		t.Errorf("an unreadable seal outside the scope: err = %v, want ErrCandidateNotFound, never a 409 that leaks it", err)
	}
}
