package httpapi

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
An area classifies the tools of its own servers, under the platform's ceiling
(NT-019, phase 2 writers).

Through the real store: `level: owner` is authorized at the scope of the
tool's server, refused for the platform's servers, a sibling's, and a tool
nobody published alike, and the platform's ruling and its withdrawal stay the
platform's administration's.
*/

// noOwnerRulings is the owner's door on a fake Curator that holds no owners.
type noOwnerRulings struct{}

func (noOwnerRulings) OwnerOf(context.Context, domain.ToolID) (admin.ToolOwner, error) {
	return admin.ToolOwner{}, admin.ErrNoOwnerScope
}

func (noOwnerRulings) ClassifyAsOwner(context.Context, admin.ToolOwner, domain.ToolClassification) error {
	return admin.ErrNoOwnerScope
}

func (noOwnerRulings) Withdraw(context.Context, domain.UserID, domain.Scope, domain.ToolID) error {
	return admin.ErrRulingUnknown
}

type classifyWorld struct {
	api     *Server
	pool    *pgxpool.Pool
	curator *admin.Curator
}

// newClassifyWorld holds cx's cx-jira, finance's fin-jira and the
// installation's github, and one published tool of each.
func newClassifyWorld(t *testing.T) classifyWorld {
	t.Helper()
	pool := testdb.Isolated(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	integrations := admin.NewIntegrations(pool, settings.NewStore(pool, v))
	allowAcmeEgress(t, integrations)
	curator := admin.NewCurator(pool)
	w := classifyWorld{
		api:  NewServer(ledger.NewMemory(), "test").WithAdministration(curator, curator, integrations),
		pool: pool, curator: curator,
	}
	putMCP(t, w.api, as(domain.RoleCurator), "cx-jira", atCX, false)
	putMCP(t, w.api, as(domain.RoleCurator), "fin-jira", atFinance, false)
	putMCP(t, w.api, as(domain.RoleCurator), "github", atInstallation, false)
	if _, err := pool.Exec(t.Context(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('installation', '', '', 'tool', 'cx-jira.search', '{"server":"cx-jira","onSurface":true}', true, 'w'),
		       ('installation', '', '', 'tool', 'fin-jira.search', '{"server":"fin-jira","onSurface":true}', true, 'w'),
		       ('installation', '', '', 'tool', 'github.search', '{"server":"github","onSurface":true}', true, 'w')`); err != nil {
		t.Fatalf("publish the tools: %v", err)
	}
	return w
}

func classifyAt(level openapi.RulingLevel, tool string, effect openapi.Effect) openapi.ClassifyToolRequestObject {
	body := &openapi.ClassifyToolJSONRequestBody{Effect: effect}
	if level != "" {
		body.Level = &level
	}
	return openapi.ClassifyToolRequestObject{ToolId: tool, Body: body}
}

// status names a ClassifyTool answer, and the 403's detail.
func classifyStatus(t *testing.T, resp openapi.ClassifyToolResponseObject) (string, string) {
	t.Helper()
	switch r := resp.(type) {
	case openapi.ClassifyTool204Response:
		return "204", ""
	case openapi.ClassifyTool400ApplicationProblemPlusJSONResponse:
		return "400", valueOr(r.Detail)
	case openapi.ClassifyTool403ApplicationProblemPlusJSONResponse:
		return "403", valueOr(r.Detail)
	case openapi.ClassifyTool409ApplicationProblemPlusJSONResponse:
		return "409", valueOr(r.Detail)
	}
	t.Fatalf("ClassifyTool = %T", resp)
	return "", ""
}

// storedAt is where the rulings on tool are stored, as kind/company/area.
func (w classifyWorld) storedAt(t *testing.T, tool string) []string {
	t.Helper()
	rows, err := w.pool.Query(context.Background(), `
		select scope_kind || '/' || company_id || '/' || area_id from settings
		where kind = 'tool_classification' and name = $1 order by 1`, tool)
	if err != nil {
		t.Fatalf("read rulings: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var at string
		if err := rows.Scan(&at); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, at)
	}
	return out
}

func TestClassifyTool_ownerLevel_isAuthorizedAtTheToolsServersScope(t *testing.T) {
	w := newClassifyWorld(t)
	areaCurator, companyCurator := grantedAt(domain.RoleCurator, cx), grantedAt(domain.RoleCurator, acmeOnly)
	platformCurator, areaAuthor := as(domain.RoleCurator), grantedAt(domain.RoleAuthor, cx)
	owner, platform := openapi.RulingLevelOwner, openapi.RulingLevelPlatform
	for _, one := range []struct {
		name   string
		caller context.Context
		level  openapi.RulingLevel
		tool   string
		want   string
		stored string
	}{
		{"area curator, own server", areaCurator, owner, "cx-jira.search", "204", "area/acme/cx"},
		{"area curator, sibling's server", areaCurator, owner, "fin-jira.search", "403", ""},
		{"area curator, the platform's server", areaCurator, owner, "github.search", "403", ""},
		{"area curator, a tool nobody published", areaCurator, owner, "nobody.search", "403", ""},
		{"area curator, the platform's level", areaCurator, platform, "cx-jira.search", "403", "area/acme/cx"},
		{"area curator, no level is the platform's", areaCurator, "", "cx-jira.search", "403", "area/acme/cx"},
		{"area author, own server", areaAuthor, owner, "cx-jira.search", "403", "area/acme/cx"},
		{"company curator, its area's server", companyCurator, owner, "fin-jira.search", "204", "area/acme/finance"},
		{"platform curator, owner of an area's server", platformCurator, owner, "cx-jira.search", "204", "area/acme/cx"},
		{"platform curator, owner of its own server", platformCurator, owner, "github.search", "204", "installation//"},
		{"platform curator, the platform's level", platformCurator, platform, "fin-jira.search", "204", "area/acme/finance installation//"},
		{"platform curator, owner of a tool nobody published", platformCurator, owner, "nobody.search", "400", ""},
	} {
		resp, err := w.api.ClassifyTool(one.caller, classifyAt(one.level, one.tool, openapi.EffectRead))
		if err != nil {
			t.Fatalf("%s: %v", one.name, err)
		}
		if got, _ := classifyStatus(t, resp); got != one.want {
			t.Errorf("%s: %s, want %s", one.name, got, one.want)
		}
		if one.stored == "" {
			continue
		}
		got := ""
		for i, at := range w.storedAt(t, one.tool) {
			if i > 0 {
				got += " "
			}
			got += at
		}
		if got != one.stored {
			t.Errorf("%s: rulings at %q, want %q", one.name, got, one.stored)
		}
	}
}

// An area hears the same about a sibling's server, the platform's and a tool
// nobody published: nothing about which it was.
func TestClassifyTool_ownerLevel_outOfReach_isOneRefusal(t *testing.T) {
	w := newClassifyWorld(t)
	details := map[string]string{}
	for _, tool := range []string{"fin-jira.search", "github.search", "nobody.search"} {
		resp, err := w.api.ClassifyTool(grantedAt(domain.RoleCurator, cx),
			classifyAt(openapi.RulingLevelOwner, tool, openapi.EffectRead))
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		status, detail := classifyStatus(t, resp)
		if status != "403" {
			t.Fatalf("%s: %s, want 403", tool, status)
		}
		details[tool] = detail
	}
	if details["fin-jira.search"] != details["nobody.search"] || details["github.search"] != details["nobody.search"] {
		t.Errorf("refusals differ: %v", details)
	}
}

// moveAfterOwnerOf answers where the tool's server is, then removes it and
// stores a server of the same name in a sibling area before the door writes.
type moveAfterOwnerOf struct {
	*admin.Curator
	t    *testing.T
	pool *pgxpool.Pool
}

func (m moveAfterOwnerOf) OwnerOf(ctx context.Context, tool domain.ToolID) (admin.ToolOwner, error) {
	owner, err := m.Curator.OwnerOf(ctx, tool)
	if _, moveErr := m.pool.Exec(ctx, `
		delete from settings where kind = 'mcp_server' and name = 'cx-jira';
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'finance', 'mcp_server', 'cx-jira', '{"transport":"http"}', true, 'usr_fin')`); moveErr != nil {
		m.t.Fatalf("move the server: %v", moveErr)
	}
	return owner, err
}

func TestClassifyTool_ownerLevel_aServerMovedBeforeTheWrite_getsNoRuling(t *testing.T) {
	w := newClassifyWorld(t)
	moving := moveAfterOwnerOf{Curator: w.curator, t: t, pool: w.pool}
	api := NewServer(ledger.NewMemory(), "test").WithAdministration(moving, w.curator, nil)
	resp, err := api.ClassifyTool(grantedAt(domain.RoleCurator, cx),
		classifyAt(openapi.RulingLevelOwner, "cx-jira.search", openapi.EffectRead))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if got, _ := classifyStatus(t, resp); got != "409" {
		t.Errorf("ClassifyTool = %s, want 409", got)
	}
	if got := w.storedAt(t, "cx-jira.search"); len(got) != 0 {
		t.Errorf("rulings at %v, want none: the finance server got cx's ruling", got)
	}
}

func withdraw(t *testing.T, api *Server, caller context.Context, tool string) string {
	t.Helper()
	resp, err := api.WithdrawToolClassification(caller, openapi.WithdrawToolClassificationRequestObject{
		ToolId: tool, Params: openapi.WithdrawToolClassificationParams{Level: openapi.WithdrawToolClassificationParamsLevelPlatform},
	})
	if err != nil {
		t.Fatalf("WithdrawToolClassification: %v", err)
	}
	switch resp.(type) {
	case openapi.WithdrawToolClassification204Response:
		return "204"
	case openapi.WithdrawToolClassification403ApplicationProblemPlusJSONResponse:
		return "403"
	case openapi.WithdrawToolClassification404ApplicationProblemPlusJSONResponse:
		return "404"
	case openapi.WithdrawToolClassification400ApplicationProblemPlusJSONResponse:
		return "400"
	}
	t.Fatalf("WithdrawToolClassification = %T", resp)
	return ""
}

// Withdrawing the ceiling is the platform's administration's alone, and hands
// the tool back to its owner's ruling.
func TestWithdrawToolClassification_needsThePlatform_andLeavesTheOwnersRuling(t *testing.T) {
	w := newClassifyWorld(t)
	for _, req := range []struct {
		caller context.Context
		level  openapi.RulingLevel
		effect openapi.Effect
	}{
		{grantedAt(domain.RoleCurator, cx), openapi.RulingLevelOwner, openapi.EffectRead},
		{as(domain.RoleCurator), openapi.RulingLevelPlatform, openapi.EffectWrite},
	} {
		if resp, err := w.api.ClassifyTool(req.caller, classifyAt(req.level, "cx-jira.search", req.effect)); err != nil {
			t.Fatalf("ClassifyTool: %v", err)
		} else if got, detail := classifyStatus(t, resp); got != "204" {
			t.Fatalf("ClassifyTool %s = %s %s", req.level, got, detail)
		}
	}
	for name, caller := range map[string]context.Context{
		"the area's curator":  grantedAt(domain.RoleCurator, cx),
		"the company's admin": grantedAt(domain.RoleAdmin, acmeOnly),
	} {
		if got := withdraw(t, w.api, caller, "cx-jira.search"); got != "403" {
			t.Errorf("%s withdrew the platform's ruling: %s, want 403", name, got)
		}
	}
	if got := withdraw(t, w.api, as(domain.RoleCurator), "cx-jira.search"); got != "204" {
		t.Fatalf("the platform's curator: %s, want 204", got)
	}
	if got := w.storedAt(t, "cx-jira.search"); len(got) != 1 || got[0] != "area/acme/cx" {
		t.Errorf("rulings at %v, want the owner's alone", got)
	}
	if got := withdraw(t, w.api, as(domain.RoleCurator), "cx-jira.search"); got != "404" {
		t.Errorf("withdrawing again: %s, want 404", got)
	}
}

// The listing says each ruling's level and where it is stored, and marks the
// owner's under a platform ruling as overridden.
func TestListTools_reportsEachRulingsLevel_andTheOverriddenOwner(t *testing.T) {
	w := newClassifyWorld(t)
	for _, req := range []struct {
		caller context.Context
		level  openapi.RulingLevel
		effect openapi.Effect
	}{
		{grantedAt(domain.RoleCurator, cx), openapi.RulingLevelOwner, openapi.EffectRead},
		{as(domain.RoleCurator), openapi.RulingLevelPlatform, openapi.EffectWrite},
	} {
		if _, err := w.api.ClassifyTool(req.caller, classifyAt(req.level, "cx-jira.search", req.effect)); err != nil {
			t.Fatalf("ClassifyTool: %v", err)
		}
	}
	resp, err := w.api.ListTools(grantedAt(domain.RoleAuthor, cx), openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	page, ok := resp.(openapi.ListTools200JSONResponse)
	if !ok {
		t.Fatalf("ListTools = %T", resp)
	}
	for _, tool := range page.Items {
		if tool.ToolId != "cx-jira.search" {
			continue
		}
		if tool.Effect != openapi.ToolEffectWrite || tool.Rulings == nil || len(*tool.Rulings) != 2 {
			t.Fatalf("listed %s with %+v, want the platform's write and two rulings", tool.Effect, tool.Rulings)
		}
		p, o := (*tool.Rulings)[0], (*tool.Rulings)[1]
		if p.Level != openapi.RulingLevelPlatform || !p.InForce || p.Scope.Company != string(domain.Installation) {
			t.Errorf("platform's ruling listed as %+v", p)
		}
		if o.Level != openapi.RulingLevelOwner || o.InForce || o.Overridden == nil || !*o.Overridden ||
			o.Scope != (openapi.Scope{Company: "acme", Area: "cx"}) || o.Effect != openapi.EffectRead {
			t.Errorf("owner's ruling listed as %+v, want overridden by the platform at acme/cx", o)
		}
		return
	}
	t.Error("cx-jira.search is not listed")
}
