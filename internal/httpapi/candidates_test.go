package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The candidate routes answer 404 outside the sealed scope, in both directions (NT-018,
"Tests that must exist"): a cx author cannot see a candidate sealed in finance, a finance
author cannot see one sealed in cx, and agent:read is not enough to see either.
*/

const routeCandidate = `---
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

Sealed in %s.
`

type candidateRoutes struct {
	pool    *pgxpool.Pool
	content *ledger.Content
	server  *Server
	agent   string
}

func newCandidateRoutes(t *testing.T) candidateRoutes {
	t.Helper()
	pool := testdb.Pool(t)
	content := ledger.NewContent(pool)
	return candidateRoutes{
		pool: pool, content: content,
		server: NewServer(ledger.NewMemory(), "test").WithCandidates(spec.NewCandidateReader(pool, content)),
		agent:  "route-cand-" + time.Now().Format("150405.000000000"),
	}
}

func (c candidateRoutes) seal(t *testing.T, area string) (string, string) {
	t.Helper()
	ctx := context.Background()
	text := []byte(fmt.Sprintf(routeCandidate, c.agent, area, area))
	parsed, err := spec.Parse("c.agent.md", text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sum := sha256.Sum256(text)
	if _, err := c.pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)`,
		c.agent, string(parsed.Version), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	var id string
	if err := c.pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ref, err := c.content.PutFor(ctx, "authoring", id, 0, text)
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := c.pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, 'acme', $4, $5, gen_random_uuid(), 'rev', 'usr_ana')`,
		id, c.agent, string(parsed.Version), area, ref); err != nil {
		t.Fatalf("seal: %v", err)
	}
	return id, string(parsed.Version)
}

func (c candidateRoutes) listed(t *testing.T, ctx context.Context) []string {
	t.Helper()
	resp, err := c.server.ListAgentCandidates(ctx, openapi.ListAgentCandidatesRequestObject{AgentId: c.agent})
	if err != nil {
		t.Fatalf("ListAgentCandidates: %v", err)
	}
	ok, isOK := resp.(openapi.ListAgentCandidates200JSONResponse)
	if !isOK {
		t.Fatalf("list answered %T", resp)
	}
	var seals []string
	for _, item := range ok.Items {
		seals = append(seals, item.SealId)
	}
	return seals
}

func (c candidateRoutes) read(t *testing.T, ctx context.Context, version string) openapi.GetAgentCandidateResponseObject {
	t.Helper()
	resp, err := c.server.GetAgentCandidate(ctx, openapi.GetAgentCandidateRequestObject{AgentId: c.agent, Version: version})
	if err != nil {
		t.Fatalf("GetAgentCandidate: %v", err)
	}
	return resp
}

func TestCandidateRoutes_eachSealIsVisibleOnlyInItsSealedScope_inBothDirections(t *testing.T) {
	c := newCandidateRoutes(t)
	cxSeal, cxVersion := c.seal(t, "cx")
	financeSeal, financeVersion := c.seal(t, "finance")

	for _, who := range []struct {
		name          string
		ctx           context.Context
		seal, version string
		hidden        string
	}{
		{"a cx author", grantedAt(domain.RoleAuthor, cx), cxSeal, cxVersion, financeVersion},
		{"a finance author", grantedAt(domain.RoleAuthor, finance), financeSeal, financeVersion, cxVersion},
	} {
		t.Run(who.name, func(t *testing.T) {
			if got := c.listed(t, who.ctx); len(got) != 1 || got[0] != who.seal {
				t.Errorf("listed %v, want only the seal of their own scope %s", got, who.seal)
			}
			if resp, ok := c.read(t, who.ctx, who.version).(openapi.GetAgentCandidate200JSONResponse); !ok ||
				resp.SealId != who.seal || resp.Definition == "" {
				t.Errorf("reading their candidate answered %T %+v", c.read(t, who.ctx, who.version), resp)
			}
			if resp := c.read(t, who.ctx, who.hidden); !isCandidate404(resp) {
				t.Errorf("reading the other scope's candidate answered %T, want 404", resp)
			}
		})
	}
	t.Run("agent:read alone", func(t *testing.T) {
		ctx := grantedAt(domain.RoleApprover, cx)
		if got := c.listed(t, ctx); len(got) != 0 {
			t.Errorf("listed %v, want nothing", got)
		}
		if resp := c.read(t, ctx, cxVersion); !isCandidate404(resp) {
			t.Errorf("reading answered %T, want 404", resp)
		}
	})
}

func TestCandidateRoutes_anUnreadableCandidate_is409OnlyToWhoMaySeeIt(t *testing.T) {
	c := newCandidateRoutes(t)
	seal, version := c.seal(t, "cx")
	tx, err := c.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.content.EraseOwnersTx(context.Background(), tx, "authoring", []string{seal}, "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	resp, ok := c.read(t, grantedAt(domain.RoleAuthor, cx), version).(openapi.GetAgentCandidate409ApplicationProblemPlusJSONResponse)
	if !ok || resp.Type == nil || *resp.Type != string(CodeCandidateUnreadable) {
		t.Errorf("a cx author got %T %+v, want 409 %s", resp, resp, CodeCandidateUnreadable)
	}
	if other := c.read(t, grantedAt(domain.RoleAuthor, finance), version); !isCandidate404(other) {
		t.Errorf("a finance author got %T, want the 404 that reveals nothing", other)
	}
}

func isCandidate404(resp openapi.GetAgentCandidateResponseObject) bool {
	_, ok := resp.(openapi.GetAgentCandidate404ApplicationProblemPlusJSONResponse)
	return ok
}
