package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
GET /work-items/trail (#204, slice 1): a card's runs and roles, narrowed by
the caller's own grants on every page.
*/

const trailCard = "github:fuseone-io/fuseone-agents#204"

var (
	trailFactory = domain.Scope{Company: "acme", Area: "factory"}
	trailCX      = domain.Scope{Company: "acme", Area: "cx"}
	trailAt      = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
)

type trailSeed struct {
	t     *testing.T
	store *ledger.Memory
}

func newTrailSeed(t *testing.T) trailSeed {
	t.Helper()
	return trailSeed{t: t, store: ledger.NewMemory()}
}

// step seals one step of a run in its scope, by its agent.
func (s trailSeed) step(id domain.RunID, agent domain.AgentID, scope domain.Scope, kind domain.StepKind, payload any) domain.Step {
	s.t.Helper()
	st := domain.Step{RunID: id, Kind: kind, Scope: scope, AgentID: agent, VersionID: "v1", At: trailAt}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			s.t.Fatalf("marshal: %v", err)
		}
		st.Payload = raw
	}
	sealed, err := s.store.Append(context.Background(), st)
	if err != nil {
		s.t.Fatalf("append %s to %s: %v", kind, id, err)
	}
	return sealed
}

// claim opens a run that claims ref, minute minutes after trailAt, and
// spends micros.
func (s trailSeed) claim(id domain.RunID, agent domain.AgentID, scope domain.Scope, ref string, micros int64) {
	s.t.Helper()
	st := domain.Step{RunID: id, Kind: domain.StepRunStarted, Scope: scope, AgentID: agent, VersionID: "v1", At: trailAt}
	if _, err := s.store.Append(context.Background(), st); err != nil {
		s.t.Fatalf("open %s: %v", id, err)
	}
	s.step(id, agent, scope, domain.StepWorkItemClaimed, domain.WorkItemClaimedPayload{Ref: domain.WorkItemRef(ref)})
	spent := domain.Step{RunID: id, Kind: domain.StepPlanned, Scope: scope, AgentID: agent, VersionID: "v1",
		At: trailAt, Cost: domain.Cost{Micros: micros}}
	if _, err := s.store.Append(context.Background(), spent); err != nil {
		s.t.Fatalf("spend on %s: %v", id, err)
	}
}

// companyAuditor reads every area of acme.
func companyAuditor() context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleAuditor}},
	})
}

func trailRequest(ref string, cursor *string, limit int) openapi.GetWorkItemTrailRequestObject {
	req := openapi.GetWorkItemTrailRequestObject{Params: openapi.GetWorkItemTrailParams{Ref: ref, Cursor: cursor}}
	if limit > 0 {
		req.Params.Limit = &limit
	}
	return req
}

func trailPage(t *testing.T, resp openapi.GetWorkItemTrailResponseObject, err error) openapi.WorkItemTrail {
	t.Helper()
	if err != nil {
		t.Fatalf("GetWorkItemTrail: %v", err)
	}
	page, ok := resp.(openapi.GetWorkItemTrail200JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want 200", resp, resp)
	}
	return openapi.WorkItemTrail(page)
}

func trailRunIDs(p openapi.WorkItemTrail) []string {
	out := []string{}
	for _, r := range p.Runs {
		out = append(out, r.RunId)
	}
	return out
}

func trailGet(t *testing.T, server *Server, ctx context.Context, req openapi.GetWorkItemTrailRequestObject) openapi.WorkItemTrail {
	t.Helper()
	resp, err := server.GetWorkItemTrail(ctx, req)
	return trailPage(t, resp, err)
}

func trailJSON(t *testing.T, resp openapi.GetWorkItemTrailResponseObject) []byte {
	t.Helper()
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func encodeRaw(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func trailServer(seed trailSeed) *Server {
	return NewServer(seed.store, "test").WithWorkItems(seed.store)
}

// Contract 4: the reference is normalised by the claim's own parser, so a
// mixed-case owner and repository find the lower-case claims.
func TestGetWorkItemTrail_aMixedCaseRef_findsTheLowerCaseClaims(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "triage", trailFactory, trailCard, 10)

	page := trailGet(t, trailServer(seed), inArea("factory", domain.RoleApprover),
		trailRequest("  github:FuseOne-IO/FuseOne-Agents#204 ", nil, 0))

	if page.Ref != trailCard || len(page.Runs) != 1 || page.Runs[0].Link != openapi.WorkItemRunLink("claim") {
		t.Fatalf("trail = ref %q, runs %v; want the lower-case claim", page.Ref, trailRunIDs(page))
	}
}

// Contract 4, and the refinement: missing (for the router), empty and every
// form the claim's parser refuses are 400 — malformed input, which says
// nothing about whether anything exists.
func TestGetWorkItemTrail_aRefTheClaimWouldRefuse_is400(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "triage", trailFactory, trailCard, 10)

	for _, ref := range []string{
		"", "   ",
		"github:fuseone-io/fuseone-agents#0204",
		"https://github.com/fuseone-io/fuseone-agents/issues/204",
		"fuseone-io/fuseone-agents#204",
		"github:fuseone-io/fuseone-agents#204 and more",
		"GITHUB:fuseone-io/fuseone-agents#204",
	} {
		resp, err := trailServer(seed).GetWorkItemTrail(inArea("factory", domain.RoleApprover), trailRequest(ref, nil, 0))
		if _, bad := resp.(openapi.GetWorkItemTrail400ApplicationProblemPlusJSONResponse); err != nil || !bad {
			t.Errorf("ref %q = %T, %v; want 400", ref, resp, err)
		}
	}
}

func TestGetWorkItemTrail_aMalformedOrTamperedCursor_is400(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "triage", trailFactory, trailCard, 10)
	smuggled := `{"t":"2026-10-01T09:00:00Z","r":"run-1","scopes":[{"company":"*"}]}`

	for _, cursor := range []string{"%%%", "bm9wZQ", encodeRaw(smuggled)} {
		resp, err := trailServer(seed).GetWorkItemTrail(inArea("factory", domain.RoleApprover), trailRequest(trailCard, &cursor, 0))
		if _, bad := resp.(openapi.GetWorkItemTrail400ApplicationProblemPlusJSONResponse); err != nil || !bad {
			t.Errorf("cursor %q = %T, %v; want 400", cursor, resp, err)
		}
	}
}

// Contract 1: a run the caller cannot read is absent, and neither its
// approvals nor its cost count in the role it shares with a readable run.
func TestGetWorkItemTrail_aRunTheCallerCannotRead_isAbsentAndCountsNowhere(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-factory", "developer", trailFactory, trailCard, 100)
	seed.step("run-factory", "developer", trailFactory, domain.StepRunFinished, nil)
	seed.claim("run-cx", "developer", trailCX, trailCard, 9_000)
	req := seed.step("run-cx", "developer", trailCX, domain.StepApprovalRequested, domain.ApprovalRequestedPayload{Tool: "x"})
	seed.step("run-cx", "developer", trailCX, domain.StepApprovalDecided, domain.ApprovalDecidedPayload{Approved: true, By: "ana", AtSeq: req.Seq})

	page := trailGet(t, trailServer(seed), inArea("factory", domain.RoleApprover), trailRequest(trailCard, nil, 0))

	if ids := trailRunIDs(page); len(ids) != 1 || ids[0] != "run-factory" {
		t.Fatalf("runs = %v, want only run-factory", ids)
	}
	roles := *page.Roles
	if len(roles) != 1 || roles[0].Runs != 1 || roles[0].ApprovalsRequested != 0 || roles[0].Approved != 0 ||
		roles[0].CostMicros == nil || *roles[0].CostMicros != 100 {
		t.Fatalf("roles = %+v; the unreadable run counted", roles)
	}
}

// A reference nobody claimed and one claimed only where the caller cannot
// read answer the same bytes: same status, same shape, nothing that differs.
func TestGetWorkItemTrail_nonexistentAndUnreadable_answerByteIdenticalBodies(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	server := trailServer(seed)
	ask := func() []byte {
		resp, err := server.GetWorkItemTrail(inArea("factory", domain.RoleApprover), trailRequest(trailCard, nil, 0))
		trailPage(t, resp, err)
		return trailJSON(t, resp)
	}

	nonexistent := ask()
	seed.claim("run-cx", "developer", trailCX, trailCard, 9_000)
	unreadable := ask()

	if !bytes.Equal(nonexistent, unreadable) {
		t.Fatalf("bodies differ:\n nonexistent %s\n unreadable  %s", nonexistent, unreadable)
	}
}

// Contract 2: the role's cost is there with cost:read and absent — the field
// itself, not a zero — without it.
func TestGetWorkItemTrail_cost_presentWithCostReadAbsentWithout(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "triage", trailFactory, trailCard, 250)
	server, ctx := trailServer(seed), inArea("factory", domain.RoleApprover)

	with := trailGet(t, server, ctx, trailRequest(trailCard, nil, 0))
	if r := (*with.Roles)[0]; r.CostMicros == nil || *r.CostMicros != 250 {
		t.Errorf("with cost:read: cost = %v, want 250", r.CostMicros)
	}

	// No shipped role reads runs without cost:read, so the caller's access
	// is stated directly at the seam the handler derives it into.
	noCost := trailAccess{runs: []domain.Scope{trailFactory}}
	resp, err := server.workItemTrail(ctx, noCost, trailCard, domain.WorkItemPage{})
	trailPage(t, resp, err)
	if raw := trailJSON(t, resp); strings.Contains(string(raw), "costMicros") {
		t.Errorf("without cost:read the body carries a cost: %s", raw)
	}
}

// Contract 3: counts and identifiers only. Nothing a step or an approval
// carries — arguments, reasons, notes — reaches the body.
func TestGetWorkItemTrail_carriesNoArgumentOrPayloadText(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "developer", trailFactory, trailCard, 10)
	seed.step("run-1", "developer", trailFactory, domain.StepToolCalled,
		map[string]any{"tool": "github.issue_write", "args": map[string]string{"body": "SECRET-ARG-cpf-123"}})
	req := seed.step("run-1", "developer", trailFactory, domain.StepApprovalRequested, domain.ApprovalRequestedPayload{
		Tool: "github.issue_write", Rule: "SECRET-RULE", Reason: "SECRET-REASON",
		ArgsRef: "content:SECRET-ARGS-REF", ArgsDigest: "SECRET-DIGEST",
	})
	seed.step("run-1", "developer", trailFactory, domain.StepApprovalDecided,
		domain.ApprovalDecidedPayload{Approved: false, By: "ana", Note: "SECRET-NOTE", AtSeq: req.Seq})
	seed.step("run-1", "developer", trailFactory, domain.StepAbandoned, domain.AbandonedPayload{By: "ana", Reason: "SECRET-ABANDON"})

	resp, err := trailServer(seed).GetWorkItemTrail(inArea("factory", domain.RoleApprover), trailRequest(trailCard, nil, 0))
	page := trailPage(t, resp, err)
	if r := (*page.Roles)[0]; r.ApprovalsRequested != 1 || r.Refused != 1 || r.Abandoned != 1 {
		t.Fatalf("role = %+v; want the counts", r)
	}
	if raw := trailJSON(t, resp); strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "issue_write") {
		t.Fatalf("the body carries step or approval text: %s", raw)
	}
}

// The cursor never carries authorization: a position taken by a caller who
// reads the whole company, resumed by one who reads an area without
// cost:read, yields only that area's runs and no cost.
func TestGetWorkItemTrail_aWideCallersCursor_reusedNarrow_readsOnlyTheNarrowScope(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "triage", trailFactory, trailCard, 10)
	seed.step("run-1", "triage", trailFactory, domain.StepRunFinished, nil)
	seed.claim("run-2", "triage", trailCX, trailCard, 20)
	seed.claim("run-3", "developer", trailCX, trailCard, 30)
	seed.claim("run-4", "reviewer", trailFactory, trailCard, 40)
	server := trailServer(seed)
	wide := trailGet(t, server, companyAuditor(), trailRequest(trailCard, nil, 1))
	after, err := domain.DecodeWorkItemCursor(*wide.NextCursor)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	narrow := trailAccess{runs: []domain.Scope{trailFactory}}
	resp, err := server.workItemTrail(context.Background(), narrow, trailCard, domain.WorkItemPage{After: after, Limit: 10})
	page := trailPage(t, resp, err)
	if ids := trailRunIDs(page); len(ids) != 1 || ids[0] != "run-4" {
		t.Fatalf("resumed narrow = %v, want only run-4", ids)
	}
	if raw := trailJSON(t, resp); strings.Contains(string(raw), "costMicros") {
		t.Errorf("the narrow page carries a cost: %s", raw)
	}
	viaRoute := trailGet(t, server, inArea("factory", domain.RoleApprover),
		trailRequest(trailCard, wide.NextCursor, 10))
	if ids := trailRunIDs(viaRoute); len(ids) != 1 || ids[0] != "run-4" {
		t.Fatalf("resumed through the route = %v, want only run-4", ids)
	}
}

func TestGetWorkItemTrail_holdingRunReadNowhere_isRefused(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)

	resp, err := trailServer(seed).GetWorkItemTrail(context.Background(), trailRequest(trailCard, nil, 0))
	if _, refused := resp.(openapi.GetWorkItemTrail403ApplicationProblemPlusJSONResponse); err != nil || !refused {
		t.Fatalf("response = %T, %v; want 403", resp, err)
	}
}

// The agent:read rule (#230), beside run:read: a run is read only where the
// caller holds both, so no agent id the caller may not read can appear.
func TestTrailScopes_areWhereTheCallerReadsRunsAndAgentsBoth(t *testing.T) {
	t.Parallel()
	acme := domain.Scope{Company: "acme"}
	cases := []struct {
		name        string
		runs, agent []domain.Scope
		want        []domain.Scope
	}{
		{"both everywhere", []domain.Scope{acme}, []domain.Scope{acme}, []domain.Scope{acme}},
		{"agents in one area", []domain.Scope{acme}, []domain.Scope{trailFactory}, []domain.Scope{trailFactory}},
		{"runs in one area", []domain.Scope{trailFactory}, []domain.Scope{acme}, []domain.Scope{trailFactory}},
		{"disjoint", []domain.Scope{trailCX}, []domain.Scope{trailFactory}, nil},
		{"no agents", []domain.Scope{acme}, nil, nil},
	}
	for _, c := range cases {
		got := trailScopes(c.runs, c.agent)
		if len(got) != len(c.want) || (len(got) > 0 && got[0] != c.want[0]) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// Disjoint grants read nothing: the store is not asked with an empty filter,
// which it would read as no narrowing at all.
func TestGetWorkItemTrail_noScopeReadsBothRunsAndAgents_isEmptyWithoutAskingTheStore(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "triage", trailFactory, trailCard, 10)

	resp, err := trailServer(seed).workItemTrail(context.Background(), trailAccess{}, trailCard, domain.WorkItemPage{})
	if page := trailPage(t, resp, err); len(page.Runs) != 0 {
		t.Fatalf("runs = %v; an access reaching nowhere read the whole ledger", trailRunIDs(page))
	}
}
