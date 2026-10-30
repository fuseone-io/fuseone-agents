package model_test

import (
	"testing"

	"github.com/fuseone/agents/internal/model"
)

func TestConfigured_namesTheProvidersThisInstallationKnows(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	if err := r.Register(model.Provider{
		Name: "from-env", Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:1",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Claimed and unbuildable here: the serve process holds no vault, and a
	// sealed credential is still a provider the administration area
	// configures. Refusing it at publish would refuse every fallback the
	// worker can in fact reach.
	r.SetConfigured(nil, "sealed")

	for name, want := range map[string]bool{"from-env": true, "sealed": true, "nobody": false} {
		if got := r.Configured(name); got != want {
			t.Errorf("Configured(%q) = %v, want %v", name, got, want)
		}
	}
}
