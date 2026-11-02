package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A card's trail, from the entry serve uses (#204, slice 1).

Two agents in two areas claim one card in the Postgres ledger openStore
returns; the trail is then read over HTTP through the router serve mounts,
over the API serve builds, by a caller who reads only one of the two areas.
A store left unwired answers every trail empty, a router that dropped the
query or a ledger that ignored the caller's scope would each answer
something else here — and a handler test with a fake would pass all three.
*/
func TestWorkItemTrailRoute_twoAgentsClaimOneCard_aOneAreaCallerReadsOnlyTheirs(t *testing.T) {
	dsn := testdb.Required(t)
	ctx := context.Background()
	store, err := openStore(ctx, dsn)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	// The package database outlives the run; a card of its own keeps an
	// earlier run's claims off this one's trail.
	run := time.Now().UnixNano()
	card := fmt.Sprintf("github:fuseone-io/journey-%d#204", run)
	factoryRun := domain.RunID(fmt.Sprintf("run-%d-triage", run))
	cxRun := domain.RunID(fmt.Sprintf("run-%d-developer", run))
	claimIn(t, store, factoryRun, "triage", domain.Scope{Company: "acme", Area: "factory"}, card, 120)
	claimIn(t, store, cxRun, "developer", domain.Scope{Company: "acme", Area: "cx"}, card, 9_000)

	route := apiHandlerFor(ledgerAPI(store, "test"))
	mixed := strings.Replace(card, "fuseone-io", "FuseOne-IO", 1)
	code, body := getTrail(t, route, factoryApprover(), url.Values{"ref": {mixed}})
	if code != http.StatusOK {
		t.Fatalf("GET trail = %d: %s", code, body)
	}
	var trail openapi.WorkItemTrail
	if err := json.Unmarshal(body, &trail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if trail.Ref != card || len(trail.Runs) != 1 || trail.Runs[0].RunId != string(factoryRun) {
		t.Fatalf("trail = %s; want only the factory run of %s", body, card)
	}
	if roles := *trail.Roles; len(roles) != 1 || roles[0].AgentId != "triage" ||
		roles[0].CostMicros == nil || *roles[0].CostMicros != 120 {
		t.Fatalf("roles = %s; the cx run counted, or the cost is missing", body)
	}
	if strings.Contains(string(body), string(cxRun)) || strings.Contains(string(body), "developer") {
		t.Fatalf("the body names the run the caller cannot read: %s", body)
	}
}

// Malformed input is 400 at the real router, the missing parameter included:
// the router refuses it before the handler, and must still say 400.
func TestWorkItemTrailRoute_malformedInput_is400(t *testing.T) {
	t.Parallel()
	route := apiHandlerFor(ledgerAPI(nil, "test"))
	for name, q := range map[string]url.Values{
		"missing ref":       {},
		"empty ref":         {"ref": {""}},
		"non-canonical ref": {"ref": {"https://github.com/fuseone-io/fuseone-agents/issues/204"}},
		"broken cursor":     {"ref": {"github:fuseone-io/fuseone-agents#204"}, "cursor": {"not-a-position"}},
	} {
		if code, body := getTrail(t, route, factoryApprover(), q); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s; want 400", name, code, body)
		}
	}
}

func factoryApprover() domain.Principal {
	return domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "factory"}, Role: domain.RoleApprover}},
	}
}

// claimIn opens a run in scope, claims card through the ledger's own append
// — where the claim is judged and projected — and spends micros.
func claimIn(t *testing.T, store Store, id domain.RunID, agent domain.AgentID, scope domain.Scope, card string, micros int64) {
	t.Helper()
	raw, err := json.Marshal(domain.WorkItemClaimedPayload{Ref: domain.WorkItemRef(card)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	at := time.Now().UTC()
	for _, st := range []domain.Step{
		{Kind: domain.StepRunStarted},
		{Kind: domain.StepWorkItemClaimed, Payload: raw},
		{Kind: domain.StepPlanned, Cost: domain.Cost{Micros: micros}},
	} {
		st.RunID, st.Scope, st.AgentID, st.VersionID, st.At = id, scope, agent, "v1", at
		if _, err := store.Append(context.Background(), st); err != nil {
			t.Fatalf("append %s to %s: %v", st.Kind, id, err)
		}
	}
}

func getTrail(t *testing.T, route http.Handler, caller domain.Principal, q url.Values) (int, []byte) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/work-items/trail?"+q.Encode(), nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), caller))
	rec := httptest.NewRecorder()
	route.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}
