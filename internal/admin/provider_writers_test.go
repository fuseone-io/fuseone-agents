package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A company or an area stores its own model provider (NT-019, phase 3 writers):
written at the caller's scope, its key sealed there, its name unique across
every scope, and removed at the row it names. Readers know where a row is
stored since 0.113.0.
*/

func writeAreaProvider(t *testing.T, i *admin.Integrations, name, key string) error {
	t.Helper()
	_, err := i.WriteProvider(context.Background(), admin.ProviderWrite{
		By: "usr_bia", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope,
		Provider: domain.ModelProvider{Name: name, Kind: "openai_compatible", BaseURL: "https://cx.example.com/v1", Enabled: true},
		APIKey:   key,
	})
	return err
}

func TestWriteProvider_atAnArea_isStoredThereWithItsKeySealedThere(t *testing.T) {
	i := newIntegrations(t)
	if err := writeAreaProvider(t, i, "cx-llm", "sk-cx"); err != nil {
		t.Fatalf("WriteProvider: %v", err)
	}
	provider, err := i.ProviderNamed(context.Background(), "cx-llm")
	if err != nil || provider.Scope != cxScope {
		t.Fatalf("ProviderNamed = %+v, %v; want cx-llm at %s", provider, err, cxScope)
	}
	if key, err := i.Credential(context.Background(), "cx-llm"); err != nil || key != "sk-cx" {
		t.Fatalf("Credential = %q, %v; want the area's key", key, err)
	}
}

// A provider's name is unique across scopes: the run, the spec and the
// registry carry the name and no scope.
func TestWriteProvider_aNameAnotherScopeHolds_isTaken(t *testing.T) {
	i := newIntegrations(t)
	if err := i.PutProvider(context.Background(), "usr_ana", platform,
		domain.ModelProvider{Name: "shared", Kind: "openai_compatible", BaseURL: "https://api.example.com/v1", Enabled: true}, "sk-p"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}
	if err := writeAreaProvider(t, i, "shared", "sk-cx"); !errors.Is(err, settings.ErrNameTaken) {
		t.Fatalf("WriteProvider = %v, want the name taken", err)
	}
	provider, err := i.ProviderNamed(context.Background(), "shared")
	if err != nil || provider.Scope != (domain.Scope{Company: domain.Installation}) {
		t.Fatalf("ProviderNamed = %+v, %v; want the installation's row untouched", provider, err)
	}
}

func TestRemoveProvider_atAnArea_removesThatRow(t *testing.T) {
	i := newIntegrations(t)
	if err := writeAreaProvider(t, i, "cx-llm", "sk-cx"); err != nil {
		t.Fatalf("WriteProvider: %v", err)
	}
	if err := i.RemoveProvider(context.Background(), admin.Removal{
		By: "usr_bia", Scope: cxScope, Name: "cx-llm", At: settings.ScopeArea, Stored: cxScope,
	}); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	if _, err := i.ProviderNamed(context.Background(), "cx-llm"); !errors.Is(err, admin.ErrProviderUnknown) {
		t.Fatalf("ProviderNamed = %v, want it gone", err)
	}
}

// A name another scope holds is answered before anything else this write
// could be refused for, a stale revision included (NT-019).
func TestWriteProvider_aTakenName_isAnsweredBeforeAStaleRevision(t *testing.T) {
	i := newIntegrations(t)
	if err := i.PutProvider(context.Background(), "usr_ana", platform,
		domain.ModelProvider{Name: "shared", Kind: "openai_compatible", BaseURL: "https://api.example.com/v1", Enabled: true}, "sk-p"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}
	stale := `"v1-stale"`
	expect, err := domain.ParseIfMatch(&stale)
	if err != nil {
		t.Fatalf("ParseIfMatch: %v", err)
	}
	_, err = i.WriteProvider(context.Background(), admin.ProviderWrite{
		By: "usr_bia", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope, Expect: expect,
		Provider: domain.ModelProvider{Name: "shared", Kind: "openai_compatible", BaseURL: "https://cx.example.com/v1", Enabled: true},
	})
	if !errors.Is(err, settings.ErrNameTaken) {
		t.Fatalf("WriteProvider = %v, want the name taken before the stale revision", err)
	}
}
