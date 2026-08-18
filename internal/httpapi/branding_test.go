package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

type fakeBranding struct {
	config admin.BrandingConfig
	set    admin.BrandingConfig
	err    error
}

func (f *fakeBranding) Current(context.Context) (admin.BrandingConfig, error) {
	if f.config.DisplayName == "" {
		return admin.DefaultBranding, nil
	}
	return f.config, nil
}

func (f *fakeBranding) Set(
	_ context.Context, _ domain.UserID, _ domain.Scope, config admin.BrandingConfig,
) error {
	if f.err != nil {
		return f.err
	}
	f.set = config
	return nil
}

func branded(store *fakeBranding) *Server {
	return NewServer(ledger.NewMemory(), "test").WithBranding(store)
}

func TestGetBranding_isPublicBecauseSignInNeedsIt(t *testing.T) {
	t.Parallel()

	resp, err := branded(&fakeBranding{config: admin.BrandingConfig{
		DisplayName: "Acme Agents",
		IconURL:     "https://assets.example/icon.png",
	}}).GetBranding(context.Background(), openapi.GetBrandingRequestObject{})
	if err != nil {
		t.Fatalf("GetBranding: %v", err)
	}
	got := resp.(openapi.GetBranding200JSONResponse)
	if got.DisplayName != "Acme Agents" || got.IconUrl == nil {
		t.Fatalf("branding = %+v", got)
	}
}

func TestSetAdminBranding_requiresBrandWrite(t *testing.T) {
	t.Parallel()

	store := &fakeBranding{}
	resp, err := branded(store).SetAdminBranding(as(domain.RoleApprover),
		openapi.SetAdminBrandingRequestObject{Body: &openapi.Branding{DisplayName: "Acme"}})
	if err != nil {
		t.Fatalf("SetAdminBranding: %v", err)
	}
	if _, ok := resp.(openapi.SetAdminBranding403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
	if store.set.DisplayName != "" {
		t.Fatalf("refused request still wrote %+v", store.set)
	}
}

func TestSetAdminBranding_invalidConfigIsABadRequest(t *testing.T) {
	t.Parallel()

	resp, err := branded(&fakeBranding{err: admin.ErrBrandingInvalid}).
		SetAdminBranding(as(domain.RoleCurator),
			openapi.SetAdminBrandingRequestObject{Body: &openapi.Branding{DisplayName: ""}})
	if err != nil {
		t.Fatalf("SetAdminBranding: %v", err)
	}
	if _, ok := resp.(openapi.SetAdminBranding400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
}
