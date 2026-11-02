package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
GET /work-items/trail serves the runs that inherited the card (#204, slice
3): link "event" or "github_delivery", and on each row the run's other
attribution — claimedRef on an inherited run that claimed elsewhere,
inheritedRef on a claim whose run inherited a card — and nothing of the
inheritance's source run.
*/

// inherit opens a run carrying item, in its scope, and has it spend micros.
func (s trailSeed) inherit(id domain.RunID, agent domain.AgentID, scope domain.Scope, item domain.RunWorkItem) {
	s.t.Helper()
	st := domain.Step{RunID: id, Kind: domain.StepRunStarted, Scope: scope, AgentID: agent, VersionID: "v1",
		At: trailAt, Payload: mustRaw(s.t, domain.RunStartedPayload{Trigger: "event", WorkItem: &item})}
	if _, err := s.store.Append(context.Background(), st); err != nil {
		s.t.Fatalf("open %s: %v", id, err)
	}
}

func mustRaw(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// Every way the ledger puts a run on a trail is a value the contract names.
func TestWorkItemRunLink_everyLedgerLinkIsInTheEnum(t *testing.T) {
	t.Parallel()
	for _, link := range []domain.WorkItemLink{
		domain.WorkItemLinkClaim, domain.WorkItemLinkAlias, domain.WorkItemLinkEvent, domain.WorkItemLinkGitHubDelivery,
	} {
		if !openapi.WorkItemRunLink(link).Valid() {
			t.Errorf("link %q is not in the WorkItemRun.link enum", link)
		}
	}
}

func TestGetWorkItemTrail_servesInheritedRunsWithTheirOtherAttribution(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-dev", "developer", trailFactory, trailCard, 10)
	seed.inherit("run-review", "reviewer", trailFactory, domain.RunWorkItem{
		Ref: trailCard, Source: domain.WorkItemFromEvent, FromRun: "run-dev-secret-source",
	})
	seed.step("run-review", "reviewer", trailFactory, domain.StepWorkItemClaimed,
		domain.WorkItemClaimedPayload{Ref: trailPR})
	seed.inherit("run-hook", "triage", trailFactory, domain.RunWorkItem{
		Ref: trailCard, Source: domain.WorkItemFromGitHubDelivery,
	})

	server := trailServer(seed)
	page := trailGet(t, server, companyAuditor(), trailRequest(trailCard, nil, 0))

	// One instant, so the run id orders them.
	want := []servedLink{
		{"run-dev", "claim", trailCard}, {"run-hook", "github_delivery", trailCard}, {"run-review", "event", trailCard},
	}
	if !slices.Equal(servedLinks(page), want) {
		t.Fatalf("runs = %+v, want %+v", servedLinks(page), want)
	}
	review := page.Runs[2]
	if review.ClaimedRef == nil || *review.ClaimedRef != trailPR || review.InheritedRef != nil {
		t.Fatalf("the inherited run = %+v; want claimedRef %s", review, trailPR)
	}
	if page.Runs[0].ClaimedRef != nil || page.Runs[0].InheritedRef != nil || page.Runs[1].ClaimedRef != nil {
		t.Fatalf("rows without another attribution carry one: %+v", page.Runs)
	}
	if roles := *page.Roles; len(roles) != 3 {
		t.Fatalf("roles = %+v; every inherited run counts", roles)
	}

	byPR := trailGet(t, server, companyAuditor(), trailRequest(trailPR, nil, 0))
	if len(byPR.Runs) != 1 || byPR.Runs[0].InheritedRef == nil || *byPR.Runs[0].InheritedRef != trailCard {
		t.Fatalf("the PR's trail = %+v; want the claim flagged with what it inherited", byPR.Runs)
	}

	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	// The source run is the run trail's to show, where its own grants apply.
	if strings.Contains(string(raw), "run-dev-secret-source") || strings.Contains(string(raw), "from_run") {
		t.Fatalf("the trail serves the inheritance's source: %s", raw)
	}
}
