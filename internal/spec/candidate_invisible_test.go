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
	"github.com/fuseone/agents/internal/standing"
	"github.com/fuseone/agents/internal/trigger"
)

/*
A candidate never opens a real run (NT-018, "A candidate never opens a real run").

A candidate has no row in agent_specs, so every reader of "the current version" is blind to
it by construction. This holds the fence anyway, reader by reader, so that a later change
merging the two lists breaks a test rather than production. The candidate differs from the
published version in everything these readers look at: its events, and whether it allows
standing grants.
*/

const invisibleBase = `---
id: %s
name: Triage
company: acme
area: cx
provider: openai
model: test-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
%s---

%s
`

func TestCandidate_isInvisibleToEveryReaderOfTheCurrentVersion(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	agent := domain.AgentID("invisible-" + time.Now().Format("150405.000000000"))
	listener := domain.AgentID(string(agent) + "-listener")
	registry := spec.NewRegistry(pool)
	publish := func(id domain.AgentID, extra, body string) spec.Spec {
		t.Helper()
		s, err := spec.Parse("x.agent.md", []byte(fmt.Sprintf(invisibleBase, id, extra, body)))
		if err != nil {
			t.Fatalf("Parse %s: %v", id, err)
		}
		if err := registry.Publish(ctx, s, "usr_ana", "acme"); err != nil {
			t.Fatalf("Publish %s: %v", id, err)
		}
		return s
	}
	published := publish(agent, "", "The published text.")
	publish(listener, "triggers:\n  - { type: event, on: candidate.only }\n", "Listen.")

	candidateText := fmt.Sprintf(invisibleBase, agent,
		"emits: [candidate.only]\nstanding_approvals: forbidden\n", "The candidate's text.")
	candidate, err := spec.Parse("x.agent.md", []byte(candidateText))
	if err != nil {
		t.Fatalf("Parse candidate: %v", err)
	}
	sealCandidate(t, pool, agent, candidate.Version, candidateText)

	t.Run("List", func(t *testing.T) {
		for _, all := range []bool{false, true} {
			agents, err := registry.List(ctx, domain.Scope{Company: "acme", Area: "cx"}, all)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			for _, a := range agents {
				if a.ID == agent && a.VersionID != published.Version {
					t.Errorf("List(all=%v) answered %s, want only %s", all, a.VersionID, published.Version)
				}
			}
		}
	})
	t.Run("Versions and Pin", func(t *testing.T) {
		versions, err := registry.Versions(ctx, agent)
		if err != nil {
			t.Fatalf("Versions: %v", err)
		}
		for _, v := range versions {
			if v.VersionID == candidate.Version {
				t.Errorf("Versions lists the candidate %s", candidate.Version)
			}
		}
		if _, err := trigger.Pin(versions, candidate.Version); !errors.Is(err, trigger.ErrUnknownVersion) {
			t.Errorf("Pin(candidate) = %v, want ErrUnknownVersion", err)
		}
	})
	t.Run("the event graph", func(t *testing.T) {
		emitters, err := registry.Emitters(ctx)
		if err != nil {
			t.Fatalf("Emitters: %v", err)
		}
		if len(emitters[agent]) != 0 {
			t.Errorf("Emitters[%s] = %v, want the published version's none", agent, emitters[agent])
		}
		edges, err := registry.Edges(ctx)
		if err != nil {
			t.Fatalf("Edges: %v", err)
		}
		for _, e := range edges {
			if e.From == agent {
				t.Errorf("the graph has the candidate's edge %+v", e)
			}
		}
	})
	t.Run("the current version under the graph's lock", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		current, found, err := spec.CurrentIn(ctx, tx, agent)
		if err != nil || !found || current.Version != published.Version {
			t.Errorf("CurrentIn = %s found=%v (%v), want %s", current.Version, found, err, published.Version)
		}
	})
	t.Run("standing/create", func(t *testing.T) {
		now := time.Now()
		err := standing.NewStore(pool).Create(ctx, standing.Grant{
			ID: "grant-" + string(agent), Tool: "crm.lookup", Agent: agent,
			Scope: domain.Scope{Company: "acme", Area: "cx"}, DailyCap: 1, Reason: "invisible candidate",
			CreatedBy: "usr_ana", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			t.Errorf("Create = %v, want the published version's permission, not the candidate's refusal", err)
		}
	})
}

// sealCandidate writes a seal of text the way release 2b will: claim, bytes and row.
func sealCandidate(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID, version domain.VersionID, text string) {
	t.Helper()
	ctx := context.Background()
	sum := sha256.Sum256([]byte(text))
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)`,
		string(agent), string(version), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	var id string
	if err := pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ref, err := ledger.NewContent(pool).PutFor(ctx, "authoring", id, 0, []byte(text))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, 'acme', 'cx', $4, gen_random_uuid(), 'rev', 'usr_ana')`,
		id, string(agent), string(version), ref); err != nil {
		t.Fatalf("seal: %v", err)
	}
}
