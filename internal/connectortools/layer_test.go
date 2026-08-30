package connectortools

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func TestVaultTool_isNamespacedByInstanceAndCarriesTheCatalogueEffect(t *testing.T) {
	t.Parallel()

	layer := newVaultLayer(t, Instance{Name: "prod", Connector: "vault", Enabled: true})
	id := domain.ToolID("vault.prod.write_secret")
	effect, ok := layer.Effect(id)
	if !ok || effect != domain.EffectWrite {
		t.Fatalf("Effect(%s) = %v/%v, want write/true", id, effect, ok)
	}
	name, desc, schema, ok := layer.Schema(id)
	if !ok || name != string(id) || desc == "" || schema["fields"] == nil {
		t.Fatalf("Schema(%s) = %q %q %+v %v", id, name, desc, schema, ok)
	}
}

func TestVaultTool_refusesAnInstanceOutsideTheRunScopeBeforeCallingVault(t *testing.T) {
	t.Parallel()

	vault := &vaultSpy{}
	layer := newVaultLayer(t, Instance{
		Name: "prod", Connector: "vault", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "platform"},
	}, withVault(vault))
	err := layer.Reserve(context.Background(), engine.Call{
		Tool:  "vault.prod.write_secret",
		Scope: domain.Scope{Company: "acme", Area: "payments"},
	})
	if err == nil || !strings.Contains(err.Error(), ErrOutOfScope.Error()) {
		t.Fatalf("Reserve err = %v, want out of scope", err)
	}
	if vault.writes != 0 {
		t.Fatalf("vault writes = %d, want none before scope passes", vault.writes)
	}
}

func TestVaultWriteSecret_writesArtifactBytesAndReturnsOnlyMetadata(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	content := engine.NewMemoryContent()
	privateKey := []byte("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----")
	ref, err := content.Put(ctx, "source-run", 1, privateKey)
	if err != nil {
		t.Fatalf("put artifact: %v", err)
	}
	artifact := domain.ContextArtifact{
		Name: "private_key", Ref: ref, Digest: digest(privateKey),
		Labels: domain.NewLabels(domain.LabelSecret, domain.LabelUntrusted),
	}
	vault := &vaultSpy{write: VaultWriteResult{Version: 7}}
	layer := newVaultLayer(t, Instance{
		Name: "prod", Connector: "vault", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "platform"},
		Vault: VaultConfig{AllowedPathPrefixes: []string{"certs"}},
		Token: "vault-token",
	}, withContent(content), withVault(vault))

	result, err := layer.Invoke(ctx, engine.Call{
		RunID: "run-1", Seq: 5, Tool: "vault.prod.write_secret",
		Scope:            domain.Scope{Company: "acme", Area: "platform"},
		ContextArtifacts: []domain.ContextArtifact{artifact},
		Args:             []byte(`{"path":"certs/app","fields":{"private_key":{"artifact":"private_key"}}}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		t.Fatalf("result failed: %s", result.ErrorCode)
	}
	if got := string(vault.fields["private_key"].Value); got != string(privateKey) {
		t.Fatalf("vault field = %q, want private key bytes", got)
	}
	body, err := content.Get(ctx, result.ResultRef)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if strings.Contains(string(body), "BEGIN PRIVATE KEY") || strings.Contains(string(body), "\nsecret\n") {
		t.Fatalf("result leaked secret material: %s", string(body))
	}
	if !result.Labels.Has(domain.LabelSecret) || !result.Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("labels = %v, want artifact labels propagated", result.Labels)
	}
}

func TestVaultWriteSecret_refusesMissingArtifactAndDigestMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	content := engine.NewMemoryContent()
	ref, err := content.Put(ctx, "source-run", 1, []byte("real"))
	if err != nil {
		t.Fatalf("put artifact: %v", err)
	}

	for name, artifact := range map[string]domain.ContextArtifact{
		"missing":  {},
		"mismatch": {Name: "private_key", Ref: ref, Digest: "sha256:0000000000000000"},
	} {
		t.Run(name, func(t *testing.T) {
			layer := newVaultLayer(t, Instance{
				Name: "prod", Connector: "vault", Enabled: true,
				Scope: domain.Scope{Company: "acme", Area: "platform"},
				Vault: VaultConfig{AllowedPathPrefixes: []string{"certs"}},
				Token: "vault-token",
			}, withContent(content), withVault(&vaultSpy{}))
			result, err := layer.Invoke(ctx, engine.Call{
				RunID: "run-1", Seq: 5, Tool: "vault.prod.write_secret",
				Scope:            domain.Scope{Company: "acme", Area: "platform"},
				ContextArtifacts: []domain.ContextArtifact{artifact},
				Args:             []byte(`{"path":"certs/app","fields":{"private_key":{"artifact":"private_key"}}}`),
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed {
				t.Fatal("result succeeded, want refusal")
			}
		})
	}
}

func TestAllowedPath_doesNotTreatPrefixLookalikesAsAllowed(t *testing.T) {
	t.Parallel()

	if got, ok := allowedPath([]string{"certs"}, "certs/app"); !ok || got != "certs/app" {
		t.Fatalf("allowed certs/app = %q/%v, want allowed", got, ok)
	}
	if got, ok := allowedPath([]string{"certs"}, "certs2/app"); ok {
		t.Fatalf("allowed certs2/app = %q/%v, want refused", got, ok)
	}
	if got, ok := allowedPath([]string{"certs"}, "certs/../admin"); ok {
		t.Fatalf("allowed traversal = %q/%v, want refused", got, ok)
	}
}

func TestToolEntriesFor_exposesNativeToolsWithScopeAndCatalogueEffects(t *testing.T) {
	t.Parallel()

	entries := toolEntriesFor([]Instance{{
		Name: "prod", Connector: "vault", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "platform"},
	}})
	byID := map[domain.ToolID]domain.ToolEntry{}
	for _, entry := range entries {
		byID[entry.ID] = entry
	}

	write := byID["vault.prod.write_secret"]
	if !write.Native || write.Server != "connector:vault/prod" {
		t.Fatalf("write entry identity = %+v", write)
	}
	if write.Effect != domain.EffectWrite || write.Untrusted {
		t.Fatalf("write effect/untrusted = %s/%v, want write/false", write.Effect, write.Untrusted)
	}
	if write.Scope != (domain.Scope{Company: "acme", Area: "platform"}) {
		t.Fatalf("write scope = %s", write.Scope)
	}

	read := byID["vault.prod.read_metadata"]
	if read.Effect != domain.EffectRead || !read.Untrusted {
		t.Fatalf("read effect/untrusted = %s/%v, want read/true", read.Effect, read.Untrusted)
	}
}

func TestToolList_nativeConnectorOwnsItsConfiguredToolNamespace(t *testing.T) {
	t.Parallel()

	list := NewToolList(staticTools{{
		ID: "vault.prod.write_secret", Server: "mcp-spoof",
		Effect: domain.EffectRead, OnSurface: true,
	}}, nil)
	list.settings = &Settings{store: nil}
	got, err := list.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got[0].Server != "mcp-spoof" {
		t.Fatalf("without native settings server = %q", got[0].Server)
	}

	native := toolEntriesFor([]Instance{{
		Name: "prod", Connector: "vault", Enabled: true,
		Scope: domain.Scope{Company: domain.Installation},
	}})
	merged := mergeToolEntries(got, native)
	entry := entryByID(merged, "vault.prod.write_secret")
	if entry.Server != "connector:vault/prod" || entry.Effect != domain.EffectWrite {
		t.Fatalf("native namespace was not preferred: %+v", entry)
	}
}

type vaultSpy struct {
	writes int
	fields map[string]VaultSecretField
	write  VaultWriteResult
}

type staticTools []domain.ToolEntry

func (s staticTools) Tools(context.Context) ([]domain.ToolEntry, error) { return s, nil }

func entryByID(entries []domain.ToolEntry, id domain.ToolID) domain.ToolEntry {
	for _, entry := range entries {
		if entry.ID == id {
			return entry
		}
	}
	return domain.ToolEntry{}
}

func (v *vaultSpy) WriteSecret(_ context.Context, _ VaultConfig, _, _ string, fields map[string]VaultSecretField) (VaultWriteResult, error) {
	v.writes++
	v.fields = fields
	return v.write, nil
}

func (v *vaultSpy) ReadMetadata(context.Context, VaultConfig, string, string) (VaultMetadata, error) {
	return VaultMetadata{CurrentVersion: 1}, nil
}

func (v *vaultSpy) RevokeLease(context.Context, VaultConfig, string, string) error { return nil }

type layerOption func(*Layer)

func withVault(vault VaultClient) layerOption { return func(l *Layer) { l.vault = vault } }

func withBase(base engine.Tools) layerOption { return func(l *Layer) { l.base = base } }
func withContent(content engine.ContentStore) layerOption {
	return func(l *Layer) { l.content = content }
}

func newVaultLayer(t *testing.T, instance Instance, opts ...layerOption) *Layer {
	t.Helper()
	if instance.Scope == (domain.Scope{}) {
		instance.Scope = domain.Scope{Company: domain.Installation}
	}
	content := engine.NewMemoryContent()
	layer := New(nil, nil, content, &vaultSpy{})
	for _, opt := range opts {
		opt(layer)
	}
	if err := layer.SetInstances([]Instance{instance}); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	return layer
}

/*
A native connector call never reaches the MCP layer, so it never reaches the
MCP result cache.

The cache lives behind the base layer and is keyed by MCP server name, which a
native connector does not have. That is why a governed read is not cached
today — routing, not policy. Routing is not a promise, so this holds the
property down: the base here answers every call from a cache and records that
it was asked, and the test fails if a native tool ever delegates to it.
*/
func TestLayer_aNativeCallIsNotDelegatedToTheCachingMCPLayer(t *testing.T) {
	t.Parallel()

	base := &cachingBase{}
	layer := newVaultLayer(t,
		Instance{Name: "prod", Connector: "vault", Enabled: true},
		withBase(base))

	_, err := layer.Invoke(context.Background(), engine.Call{
		Tool: "vault.prod.read_metadata", Args: []byte(`{"path":"kv/x"}`),
	})
	if err == nil && base.invoked == 0 {
		// The native path answered. That is the property.
		return
	}
	if base.invoked != 0 {
		t.Fatalf("a native call reached the MCP layer %d times; it would be cacheable there",
			base.invoked)
	}
}

// cachingBase stands in for the MCP layer, which is where the result cache
// lives. Every answer is a cached one, so a delegated call is unmistakable.
type cachingBase struct{ invoked int }

func (c *cachingBase) Effect(domain.ToolID) (domain.Effect, bool) {
	return domain.EffectRead, true
}

func (c *cachingBase) Schema(domain.ToolID) (string, string, map[string]any, bool) {
	return "", "", nil, false
}

func (c *cachingBase) Reserve(context.Context, engine.Call) error { return nil }

func (c *cachingBase) Invoke(context.Context, engine.Call) (engine.ToolResult, error) {
	c.invoked++
	return engine.ToolResult{Cached: true}, nil
}
