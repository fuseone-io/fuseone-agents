package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/memory"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/vault"
	"github.com/fuseone/agents/internal/workitem"
)

/*
From the area's door to the worker's catalogue (NT-019, phase 2 writers).

The area's curator rules on its own server's tool through the API; the worker
applies it on its next Sync. The platform's ruling then decides, a later owner
ruling changes nothing in effect, and withdrawing the platform's hands the
tool back to the owner's latest word.
*/

type ownerJourney struct {
	api     *httpapi.Server
	catalog *tools.Catalog
	curator *admin.Curator
}

func newOwnerJourney(t *testing.T) ownerJourney {
	t.Helper()
	ctx := t.Context()
	pool := testdb.Isolated(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	curator := admin.NewCurator(pool)
	integrations := admin.NewIntegrations(pool, store)
	remote := httptest.NewServer(mcpHandler())
	t.Cleanup(remote.Close)
	if err := integrations.PutMCPEgressPolicy(ctx, "usr_ana", domain.Scope{Company: "default", Area: "platform"},
		admin.MCPEgressPolicy{Scope: ownerArea, Origins: []string{mcpOrigin(t, remote.URL)}}); err != nil {
		t.Fatalf("PutMCPEgressPolicy: %v", err)
	}

	// The tool list serve hands the console, layer for layer.
	listed := workitem.NewToolList(memory.NewToolList(
		connectortools.NewToolList(curator, connectortools.NewSettings(store))))
	j := ownerJourney{
		api:     httpapi.NewServer(ledger.NewMemory(), "test").WithAdministration(curator, listed, integrations),
		catalog: tools.NewCatalog(engine.NewMemoryContent()),
		curator: curator,
	}
	t.Cleanup(func() { _ = j.catalog.Close() })

	area := openapi.ConnectorScopeKindArea
	resp, err := j.api.PutMCPServer(grantedOn(domain.RoleAdmin, ownerArea), openapi.PutMCPServerRequestObject{
		Name: "cx-ping", Body: &openapi.PutMCPServerJSONRequestBody{
			ScopeKind: &area, Company: ptrTo("acme"), Area: ptrTo("cx"),
			Transport: ptrTo(openapi.TransportHttp), Url: ptrTo(remote.URL),
		},
	})
	if _, ok := resp.(openapi.PutMCPServer204Response); err != nil || !ok {
		t.Fatalf("PutMCPServer = %#v, %v", resp, err)
	}
	newReconciler(j.catalog, integrations, recordingHealth{}).publishingTo(curator).reconcile(ctx)
	if !j.catalog.Connected("cx-ping") {
		t.Fatal("the area's server did not connect; the rulings below would prove nothing")
	}
	return j
}

var ownerArea = domain.Scope{Company: "acme", Area: "cx"}

func grantedOn(role domain.Role, scope domain.Scope) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser, Grants: []domain.Grant{{Scope: scope, Role: role}},
	})
}

// rule classifies cx-ping.ping through the door, naming the definition the
// console lists.
func (j ownerJourney) rule(t *testing.T, caller context.Context, level openapi.RulingLevel, effect openapi.Effect) {
	t.Helper()
	page, err := j.api.ListTools(caller, openapi.ListToolsRequestObject{})
	listed, ok := page.(openapi.ListTools200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("ListTools = %#v, %v", page, err)
	}
	var digest *string
	for _, tool := range listed.Items {
		if tool.ToolId == "cx-ping.ping" {
			digest = tool.Digest
		}
	}
	resp, err := j.api.ClassifyTool(caller, openapi.ClassifyToolRequestObject{
		ToolId: "cx-ping.ping", Body: &openapi.ClassifyToolJSONRequestBody{Effect: effect, Digest: digest, Level: &level},
	})
	if _, ok := resp.(openapi.ClassifyTool204Response); err != nil || !ok {
		t.Fatalf("ClassifyTool %s %s = %#v, %v", level, effect, resp, err)
	}
}

// applied is the effect the worker enforces after its next Sync.
func (j ownerJourney) applied(t *testing.T) domain.Effect {
	t.Helper()
	if _, err := j.catalog.Sync(t.Context(), j.curator, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	effect, _ := j.catalog.Effect("cx-ping.ping")
	return effect
}

func TestOwnerClassificationJourney_theWorkerAppliesTheOwnersRulingUnderTheCeiling(t *testing.T) {
	j := newOwnerJourney(t)
	areaCurator, platformCurator := grantedOn(domain.RoleCurator, ownerArea), grantedOn(domain.RoleCurator,
		domain.Scope{Company: "default", Area: "platform"})

	j.rule(t, areaCurator, openapi.RulingLevelOwner, openapi.EffectRead)
	if got := j.applied(t); got != domain.EffectRead {
		t.Fatalf("after the owner's ruling: %v, want read", got)
	}
	j.rule(t, platformCurator, openapi.RulingLevelPlatform, openapi.EffectWrite)
	if got := j.applied(t); got != domain.EffectWrite {
		t.Fatalf("after the platform's ruling: %v, want write", got)
	}
	j.rule(t, areaCurator, openapi.RulingLevelOwner, openapi.EffectDestructive)
	if got := j.applied(t); got != domain.EffectWrite {
		t.Fatalf("a later owner ruling under the ceiling: %v, want the platform's write", got)
	}

	j.withdraw(t, platformCurator)
	if got := j.applied(t); got != domain.EffectDestructive {
		t.Errorf("after the ceiling's withdrawal: %v, want the owner's destructive", got)
	}
}

func (j ownerJourney) withdraw(t *testing.T, caller context.Context) {
	t.Helper()
	resp, err := j.api.WithdrawToolClassification(caller, openapi.WithdrawToolClassificationRequestObject{
		ToolId: "cx-ping.ping", Params: openapi.WithdrawToolClassificationParams{Level: openapi.WithdrawToolClassificationParamsLevelPlatform},
	})
	if _, ok := resp.(openapi.WithdrawToolClassification204Response); err != nil || !ok {
		t.Fatalf("WithdrawToolClassification = %#v, %v", resp, err)
	}
}

// With no owner's ruling under it, a withdrawn ceiling leaves the tool with
// none, and the running worker stops it: it does not keep the withdrawn effect.
func TestOwnerClassificationJourney_aCeilingWithdrawnWithNoOwnerRuling_stopsTheTool(t *testing.T) {
	j := newOwnerJourney(t)
	platformCurator := grantedOn(domain.RoleCurator, domain.Scope{Company: "default", Area: "platform"})
	j.rule(t, platformCurator, openapi.RulingLevelPlatform, openapi.EffectWrite)
	if got := j.applied(t); got != domain.EffectWrite {
		t.Fatalf("after the platform's ruling: %v, want write", got)
	}
	j.withdraw(t, platformCurator)
	if got := j.applied(t); got != domain.EffectUnknown {
		t.Errorf("after the withdrawal: %v, want unknown", got)
	}
}
