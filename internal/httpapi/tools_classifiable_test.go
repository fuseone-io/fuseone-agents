package httpapi

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The tools a curator may rule on as the owner, asked for in one listing
(NT-019): `classifiable=true`.

A target answer holds the tools whose server's scope contains the target, so
a company's curator asking for `acme` never sees `acme/cx`'s, which they may
classify. The governed set is the other direction: the tools whose server is
stored at a scope one of the caller's tool:classify scopes contains.
*/

// classifiableFor is ListTools with classifiable=true, and target when given.
func classifiableFor(t *testing.T, s *Server, caller context.Context, target string) (string, []string) {
	t.Helper()
	req := openapi.ListToolsRequestObject{Params: openapi.ListToolsParams{Classifiable: ptr(true)}}
	if target != "" {
		req.Params.Scope = ptr(target)
	}
	resp, err := s.ListTools(caller, req)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	switch r := resp.(type) {
	case openapi.ListTools200JSONResponse:
		ids := make([]string, 0, len(r.Items))
		for _, tool := range r.Items {
			ids = append(ids, tool.ToolId)
		}
		slices.Sort(ids)
		return "200", ids
	case openapi.ListTools400ApplicationProblemPlusJSONResponse:
		return "400", nil
	case openapi.ListTools403ApplicationProblemPlusJSONResponse:
		return "403", nil
	}
	t.Fatalf("ListTools = %T", resp)
	return "", nil
}

func TestListTools_classifiable_answersWhatTheCallerMayRuleOnAsTheOwner(t *testing.T) {
	w := newTargetWorld(t)
	for _, one := range []struct {
		name   string
		caller context.Context
		want   []string
	}{
		{"a company curator: the company's and its areas'", grantedAt(domain.RoleCurator, acmeOnly),
			sorted("acme-wiki.read", "cx-jira.search", "fin-jira.search")},
		{"an area curator: its area's alone, no sibling's", grantedAt(domain.RoleCurator, cx),
			sorted("cx-jira.search")},
		{"another company's curator: nothing of acme's", grantedAt(domain.RoleCurator, domain.Scope{Company: "globex"}),
			[]string{}},
		{"the platform's curator: every tool", as(domain.RoleCurator), everyTool},
	} {
		got, ids := classifiableFor(t, w.api, one.caller, "")
		if got != "200" || !slices.Equal(ids, one.want) {
			t.Errorf("%s: %s %v, want 200 %v", one.name, got, ids, one.want)
		}
	}
}

// A reader who classifies nowhere is refused, as every tool:classify door
// refuses them; asked beside a target, the two filters are not combined.
func TestListTools_classifiable_refusals(t *testing.T) {
	w := newTargetWorld(t)
	if got, _ := classifiableFor(t, w.api, grantedAt(domain.RoleAuthor, cx), ""); got != "403" {
		t.Errorf("an author = %s, want 403", got)
	}
	if got, _ := classifiableFor(t, w.api, grantedAt(domain.RoleCurator, cx), "acme/cx"); got != "400" {
		t.Errorf("with a target = %s, want 400", got)
	}
}
