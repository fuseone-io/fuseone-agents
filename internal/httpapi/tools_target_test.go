package httpapi

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The tools an author picks from are asked for one target scope (NT-019, phase 2
writers, part B2a).

Through the real store: cx's cx-jira, finance's fin-jira, the company's
acme-wiki, the installation's github, a flag-connected server no row
describes, and the platform's own ticket answer. The answer for a target is
what an agent stored there can run, the Gate's checkScope over the same
scopes, and nothing else.
*/

const ticketAnswer = string(connectortools.TicketAnswerTool)

// newTargetWorld is newClassifyWorld with a company's server, a tool of a
// server no row holds, and the platform's own tools listed beside them, as
// serve lists them.
func newTargetWorld(t *testing.T) classifyWorld {
	t.Helper()
	w := newClassifyWorld(t)
	putMCP(t, w.api, as(domain.RoleCurator), "acme-wiki", atAcme, false)
	if _, err := w.pool.Exec(t.Context(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('installation', '', '', 'tool', 'acme-wiki.read', '{"server":"acme-wiki","onSurface":true}', true, 'w'),
		       ('installation', '', '', 'tool', 'flagged.ping', '{"server":"flagged","onSurface":true}', true, 'w')`); err != nil {
		t.Fatalf("publish the tools: %v", err)
	}
	integrations := w.api.integrations
	w.api = NewServer(ledger.NewMemory(), "test").
		WithAdministration(w.curator, connectortools.NewToolList(w.curator, nil), integrations)
	return w
}

// grantedOnEach is a caller holding role at every one of scopes.
func grantedOnEach(role domain.Role, scopes ...domain.Scope) context.Context {
	grants := make([]domain.Grant, 0, len(scopes))
	for _, s := range scopes {
		grants = append(grants, domain.Grant{Scope: s, Role: role})
	}
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser, Grants: grants,
	})
}

// listedFor is ListTools for a target ("" for none): the status, and the tool
// ids of a 200, sorted.
func listedFor(t *testing.T, s *Server, caller context.Context, target string) (string, []string) {
	t.Helper()
	req := openapi.ListToolsRequestObject{}
	if target != "" {
		req.Params.Scope = ptr(target)
	}
	resp, err := s.ListTools(caller, req)
	if err != nil {
		t.Fatalf("ListTools %q: %v", target, err)
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
	case openapi.ListTools404ApplicationProblemPlusJSONResponse:
		return "404", nil
	}
	t.Fatalf("ListTools %q = %T", target, resp)
	return "", nil
}

// sorted is ids in the order listedFor answers them.
func sorted(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

var (
	installationTools = sorted("github.search", "flagged.ping", ticketAnswer)
	cxTools           = sorted("cx-jira.search", "acme-wiki.read", "github.search", "flagged.ping", ticketAnswer)
	financeTools      = sorted("fin-jira.search", "acme-wiki.read", "github.search", "flagged.ping", ticketAnswer)
	everyTool         = sorted("cx-jira.search", "fin-jira.search", "acme-wiki.read",
		"github.search", "flagged.ping", ticketAnswer)
)

func TestListTools_forATarget_answersWhatAnAgentStoredThereCanRun(t *testing.T) {
	w := newTargetWorld(t)
	for _, one := range []struct {
		name   string
		caller context.Context
		target string
		want   []string
	}{
		{"an area author, its own area", grantedAt(domain.RoleAuthor, cx), "acme/cx", cxTools},
		// The union of the caller's scopes would offer finance's tool to a cx
		// agent, which checkScope then stops.
		{"an author of two areas, one of them", grantedOnEach(domain.RoleAuthor, cx, finance), "acme/cx", cxTools},
		// The caller's grants instead of the target would answer only what
		// contains acme, and lose cx's own.
		{"a company author, one of its areas", grantedAt(domain.RoleAuthor, acmeOnly), "acme/cx", cxTools},
		{"a company author, the company", grantedAt(domain.RoleAuthor, acmeOnly), "acme",
			sorted("acme-wiki.read", "github.search", "flagged.ping", ticketAnswer)},
		{"the platform's reader, any area", as(domain.RoleCurator), "acme/finance", financeTools},
		{"an installation-wide reader", asInstallation(domain.RoleAuthor), "acme/cx", cxTools},
	} {
		got, ids := listedFor(t, w.api, one.caller, one.target)
		if got != "200" || !slices.Equal(ids, one.want) {
			t.Errorf("%s: %s %v, want 200 %v", one.name, got, ids, one.want)
		}
	}
}

var authorHereApproverThere = auth.WithPrincipal(context.Background(), domain.Principal{
	ID: "usr_ana", Kind: domain.PrincipalUser, Grants: []domain.Grant{
		{Scope: finance, Role: domain.RoleAuthor}, {Scope: cx, Role: domain.RoleApprover},
	},
})

// A target outside every scope the caller reads tool:read in is answered as
// one that does not exist: a 403 would confirm it does.
func TestListTools_aTargetTheCallerCannotRead_is404(t *testing.T) {
	w := newTargetWorld(t)
	for _, one := range []struct {
		name   string
		caller context.Context
		target string
	}{
		{"an area author, a sibling area", grantedAt(domain.RoleAuthor, cx), "acme/finance"},
		// The grant inside the target, not the target inside the grant.
		{"an area author, its company", grantedAt(domain.RoleAuthor, cx), "acme"},
		{"a company author, another company", grantedAt(domain.RoleAuthor, acmeOnly), "globex/cx"},
		// tool:read in finance, and only a role without it in cx.
		{"an author elsewhere, approver of the target", authorHereApproverThere, "acme/cx"},
	} {
		if got, ids := listedFor(t, w.api, one.caller, one.target); got != "404" {
			t.Errorf("%s: %s %v, want 404", one.name, got, ids)
		}
	}
}

// A target is a real company or area, as stored: nothing is folded into one,
// and the installation is not a target. Judged before who may read it, so the
// caller learns about the request's form and nothing about the scope.
func TestListTools_aMalformedTarget_is400(t *testing.T) {
	w := newTargetWorld(t)
	for _, target := range []string{
		"acme/CX", "ACME/cx", " acme/cx", "acme/cx ", "acme/c x", "acme/cx/x",
		"acme/", "/cx", "*", "*/cx", "acme/crédito",
	} {
		if got, ids := listedFor(t, w.api, grantedAt(domain.RoleAuthor, cx), target); got != "400" {
			t.Errorf("%q: %s %v, want 400", target, got, ids)
		}
	}
}

// Without a target, the platform's screen lists every tool, and everybody else
// only what every scope inherits: nothing of any area is answered without
// naming it.
func TestListTools_withoutATarget_isTheInstallationsUnlessThePlatformReads(t *testing.T) {
	w := newTargetWorld(t)
	for _, one := range []struct {
		name   string
		caller context.Context
		want   []string
	}{
		{"an area author", grantedAt(domain.RoleAuthor, cx), installationTools},
		{"a company author", grantedAt(domain.RoleAuthor, acmeOnly), installationTools},
		{"the platform's reader", as(domain.RoleAuthor), everyTool},
		{"an installation-wide reader", asInstallation(domain.RoleCurator), everyTool},
	} {
		got, ids := listedFor(t, w.api, one.caller, "")
		if got != "200" || !slices.Equal(ids, one.want) {
			t.Errorf("%s: %s %v, want 200 %v", one.name, got, ids, one.want)
		}
	}
}

// Each listed tool says where its server is stored, for the console to decide
// who may rule on it (NT-019).
func TestListTools_eachToolCarriesItsServersScope(t *testing.T) {
	w := newTargetWorld(t)
	resp, err := w.api.ListTools(as(domain.RoleCurator), openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	items := resp.(openapi.ListTools200JSONResponse).Items
	found := false
	for _, tool := range items {
		if tool.ToolId != "cx-jira.search" {
			continue
		}
		found = true
		if tool.Scope == nil || tool.Scope.Company != "acme" || tool.Scope.Area != "cx" {
			t.Errorf("cx-jira.search scope = %+v, want acme/cx", tool.Scope)
		}
	}
	if !found {
		t.Fatal("cx-jira.search not listed")
	}
}
