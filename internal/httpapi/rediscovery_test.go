package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// The fake keeps the store's rules: an unknown server is not found, and one
// switched off has no worker holding it to ask.
func (f *fakeAdmin) RequestMCPRediscovery(ctx context.Context, by domain.UserID, _ domain.Scope, name string) (int64, error) {
	server, err := f.MCPServerNamed(ctx, name)
	if err != nil {
		return 0, err
	}
	if !server.Enabled {
		return 0, admin.ErrMCPServerDisabled
	}
	f.rediscovered, f.putBy = name, by
	return f.rediscoveries[name].Generation + 1, nil
}

func (f *fakeAdmin) MCPRediscovery(ctx context.Context, name string) (domain.MCPRediscoveryStatus, error) {
	if _, err := f.MCPServerNamed(ctx, name); err != nil {
		return domain.MCPRediscoveryStatus{}, err
	}
	status := f.rediscoveries[name]
	status.Server = name
	return status, nil
}

func rediscoverable() *fakeAdmin {
	return &fakeAdmin{servers: []domain.MCPServer{
		{Name: "crm", Enabled: true}, {Name: "billing", Enabled: false},
	}}
}

func TestRequestMCPRediscovery_recordsWhoAsked_andAnswersTheGeneration(t *testing.T) {
	t.Parallel()
	store := rediscoverable()
	store.rediscoveries = map[string]domain.MCPRediscoveryStatus{"crm": {Generation: 4}}

	resp, err := serverWith(t, store).RequestMCPRediscovery(as(domain.RoleCurator),
		openapi.RequestMCPRediscoveryRequestObject{Name: "crm"})

	accepted, ok := resp.(openapi.RequestMCPRediscovery202JSONResponse)
	if err != nil || !ok || accepted.Generation != 5 {
		t.Fatalf("response = %#v, %v; want 202 with generation 5", resp, err)
	}
	if store.rediscovered != "crm" || store.putBy != "usr_ana" {
		t.Errorf("request = %q by %q, want crm by the caller", store.rediscovered, store.putBy)
	}
}

func TestRequestMCPRediscovery_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()
	store := rediscoverable()

	resp, err := serverWith(t, store).RequestMCPRediscovery(as(domain.RoleAuditor),
		openapi.RequestMCPRediscoveryRequestObject{Name: "crm"})

	if _, refused := resp.(openapi.RequestMCPRediscovery403ApplicationProblemPlusJSONResponse); err != nil || !refused {
		t.Fatalf("response = %#v, %v; want a refusal", resp, err)
	}
	if store.rediscovered != "" {
		t.Errorf("a request was recorded despite the refusal: %q", store.rediscovered)
	}
}

func TestRequestMCPRediscovery_unknownOrSwitchedOff_isRefusedForWhatItIs(t *testing.T) {
	t.Parallel()
	api := serverWith(t, rediscoverable())

	unknown, _ := api.RequestMCPRediscovery(as(domain.RoleCurator),
		openapi.RequestMCPRediscoveryRequestObject{Name: "ledger"})
	off, _ := api.RequestMCPRediscovery(as(domain.RoleCurator),
		openapi.RequestMCPRediscoveryRequestObject{Name: "billing"})

	if _, ok := unknown.(openapi.RequestMCPRediscovery404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("unknown server = %#v, want 404", unknown)
	}
	refused, ok := off.(openapi.RequestMCPRediscovery400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Type == nil || *refused.Type != string(CodeInvalidInput) {
		t.Errorf("switched-off server = %#v, want 400 %s", off, CodeInvalidInput)
	}
}

func TestGetMCPRediscovery_answersEachWorkersReceipt(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	store := rediscoverable()
	store.rediscoveries = map[string]domain.MCPRediscoveryStatus{"crm": {
		Generation: 2, RequestedBy: "usr_bia", RequestedAt: at,
		Receipts: []domain.MCPRediscoveryReceipt{
			{Worker: "worker-a", Generation: 2, Succeeded: true, At: at},
			{Worker: "worker-b", Generation: 1, Succeeded: false, At: at},
		},
	}}

	resp, err := serverWith(t, store).GetMCPRediscovery(as(domain.RoleCurator),
		openapi.GetMCPRediscoveryRequestObject{Name: "crm"})

	got, ok := resp.(openapi.GetMCPRediscovery200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("response = %#v, %v; want 200", resp, err)
	}
	want := []openapi.MCPRediscoveryReceipt{
		{Worker: "worker-a", Generation: 2, Succeeded: true, At: at},
		{Worker: "worker-b", Generation: 1, Succeeded: false, At: at},
	}
	if got.Generation != 2 || got.RequestedBy == nil || *got.RequestedBy != "usr_bia" ||
		len(got.Receipts) != 2 || got.Receipts[0] != want[0] || got.Receipts[1] != want[1] {
		t.Errorf("status = %+v, want generation 2 by usr_bia with %+v", got, want)
	}
}

func TestGetMCPRediscovery_unknownServerOrNoPermission_isRefused(t *testing.T) {
	t.Parallel()
	api := serverWith(t, rediscoverable())

	unknown, _ := api.GetMCPRediscovery(as(domain.RoleCurator), openapi.GetMCPRediscoveryRequestObject{Name: "ledger"})
	auditor, _ := api.GetMCPRediscovery(as(domain.RoleAuditor), openapi.GetMCPRediscoveryRequestObject{Name: "crm"})

	if _, ok := unknown.(openapi.GetMCPRediscovery404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("unknown = %#v, want 404", unknown)
	}
	if _, ok := auditor.(openapi.GetMCPRediscovery403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("auditor = %#v, want 403", auditor)
	}
}
