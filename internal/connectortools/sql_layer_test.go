package connectortools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func TestSQLNativeTool_schemaContainsOnlyTheGovernedQuestion(t *testing.T) {
	t.Parallel()

	layer := newSQLLayer(t, &rotatingSQLIssuer{}, &recordingSQLExecutor{}, &cachingBase{})
	name, _, schema, ok := layer.Schema("sql.app-x.run_query_template")
	if !ok || name != "sql.app-x.run_query_template" {
		t.Fatalf("Schema = %q/%v, want the configured SQL tool", name, ok)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"vault", "credential", "username", "password", "dsn", "query"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Errorf("schema exposes %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"template_id", "parameters"} {
		if !strings.Contains(strings.ToLower(text), required) {
			t.Errorf("schema does not name %q: %s", required, text)
		}
	}
}

func TestSQLNativeTool_twoInvocationsUseDistinctAuthorityAndNeverTheMCPCache(t *testing.T) {
	t.Parallel()

	issuer := &rotatingSQLIssuer{}
	executor := &recordingSQLExecutor{}
	base := &cachingBase{}
	layer := newSQLLayer(t, issuer, executor, base)

	results := make(chan engine.ToolResult, 2)
	errs := make(chan error, 2)
	for i := 1; i <= 2; i++ {
		go func(seq int) {
			call := boundSQLLayerCall(layer, int64(seq))
			call.RunID = domain.RunID(fmt.Sprintf("run-sql-%d", seq))
			result, err := layer.Invoke(context.Background(), call)
			results <- result
			errs <- err
		}(i)
	}

	content := layer.content
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		result := <-results
		if result.Failed || result.Cached || result.ResultRef == "" {
			t.Fatalf("result = %+v, want live governed content", result)
		}
		if !result.Labels.Has(domain.LabelUntrusted) {
			t.Fatalf("labels = %v, want database rows marked untrusted", result.Labels)
		}
		body, err := content.Get(context.Background(), result.ResultRef)
		if err != nil {
			t.Fatalf("read result: %v", err)
		}
		leaks(t, result)
		leaks(t, body)
	}

	if base.invoked != 0 {
		t.Fatalf("SQL delegated to the MCP cache %d times", base.invoked)
	}
	issued, revoked := issuer.snapshot()
	credentials := executor.snapshot()
	if len(issued) != 2 || issued[0] == issued[1] {
		t.Fatalf("issued leases = %#v, want two distinct leases", issued)
	}
	if len(revoked) != 2 || revoked[0] == revoked[1] {
		t.Fatalf("revoked leases = %#v, want both distinct leases returned", revoked)
	}
	if len(credentials) != 2 || credentials[0] == credentials[1] {
		t.Fatalf("database credentials = %#v, want one per execution", credentials)
	}
}

func TestSQLNativeTool_refusesArgumentsOutsideTheRegisteredShape(t *testing.T) {
	t.Parallel()

	issuer := &rotatingSQLIssuer{}
	layer := newSQLLayer(t, issuer, &recordingSQLExecutor{}, &cachingBase{})
	for name, args := range map[string]string{
		"sql text":       `{"template_id":"orders_by_customer","parameters":{},"sql":"drop table users"}`,
		"missing id":     `{"parameters":{}}`,
		"trailing value": `{"template_id":"orders_by_customer","parameters":{}} true`,
	} {
		t.Run(name, func(t *testing.T) {
			call := boundSQLLayerCall(layer, 1)
			call.Args = []byte(args)
			result, err := layer.Invoke(context.Background(), call)
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
				t.Fatalf("result = %+v, want bad arguments", result)
			}
		})
	}
	if issued, _ := issuer.snapshot(); len(issued) != 0 {
		t.Fatalf("bad arguments issued %d credentials", len(issued))
	}
}

func TestSQLNativeTool_failureStoresOnlySafeAuthorityOutcomes(t *testing.T) {
	t.Parallel()

	content := engine.NewMemoryContent()
	layer := New(&cachingBase{}, nil, content, &vaultIssuer{}).
		WithSQLRuntime(failedSQLRunner{})
	if err := layer.SetInstances(ready()); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	result, err := layer.Invoke(context.Background(), boundSQLLayerCall(layer, 1))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Failed || result.ErrorCode != CodeConnectorUpstreamFailed || result.ResultRef == "" {
		t.Fatalf("result = %+v, want a referenced bounded failure", result)
	}
	body, err := content.Get(context.Background(), result.ResultRef)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	leaks(t, result)
	leaks(t, body)
	var audit SQLResult
	if err := json.Unmarshal(body, &audit); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if audit.IssuanceOutcome != IssuanceSucceeded || audit.Revocation != RevocationFailed {
		t.Fatalf("audit = %+v, want safe issuance and revocation outcomes", audit)
	}
	if len(audit.Columns) != 0 || len(audit.Rows) != 0 {
		t.Fatalf("failed query retained database data: %+v", audit)
	}
}

func newSQLLayer(
	t *testing.T, issuer CredentialIssuer, executor SQLExecutor, base engine.Tools,
) *Layer {
	t.Helper()
	instances := ready()
	content := engine.NewMemoryContent()
	runtime := NewSQLRuntime(
		NewCredentialResolver(staticConfig(instances), fixedVaultToken{}, issuer), executor)
	layer := New(base, nil, content, issuerAsVaultClient{CredentialIssuer: issuer}).WithSQLRuntime(runtime)
	if err := layer.SetInstances(instances); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	return layer
}

func sqlLayerCall(seq int64) engine.Call {
	return engine.Call{
		RunID: "run-sql", Seq: seq, Tool: "sql.app-x.run_query_template",
		Scope: runScope(),
		Args:  []byte(`{"template_id":"orders_by_customer","parameters":{"customer_id":"cus_1","since":"2026-08-30T12:00:00Z"}}`),
	}
}

func boundSQLLayerCall(layer *Layer, seq int64) engine.Call {
	call := sqlLayerCall(seq)
	call.ContractDigest = layer.ApprovalBinding(call)
	return call
}

type fixedVaultToken struct{}

func (fixedVaultToken) RevealVaultToken(context.Context, ConfiguredInstance) (string, error) {
	return vaultTokenCanary, nil
}

type rotatingSQLIssuer struct {
	mu      sync.Mutex
	next    int
	issued  []string
	revoked []string
}

func (v *rotatingSQLIssuer) IssueDatabaseCredential(
	context.Context, VaultConfig, string, string, string,
) (IssuedCredential, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.next++
	leaseID := fmt.Sprintf("%s-%d", leaseCanary, v.next)
	username := fmt.Sprintf("%s-%d", usernameCanary, v.next)
	password := fmt.Sprintf("%s-%d", credentialCanary, v.next)
	v.issued = append(v.issued, leaseID)
	return IssuedCredential{
		credential: Credential{username: username, password: password},
		leaseID:    leaseID, ttlSeconds: 300,
	}, nil
}

func (v *rotatingSQLIssuer) RevokeLease(
	_ context.Context, _ VaultConfig, _, leaseID string,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.revoked = append(v.revoked, leaseID)
	return nil
}

func (v *rotatingSQLIssuer) snapshot() ([]string, []string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.issued...), append([]string(nil), v.revoked...)
}

// issuerAsVaultClient supplies the existing Vault operations without widening
// CredentialIssuer. SQL uses only the embedded issuance methods.
type issuerAsVaultClient struct{ CredentialIssuer }

func (issuerAsVaultClient) WriteSecret(
	context.Context, VaultConfig, string, string, map[string]VaultSecretField,
) (VaultWriteResult, error) {
	return VaultWriteResult{}, nil
}

func (issuerAsVaultClient) ReadMetadata(
	context.Context, VaultConfig, string, string,
) (VaultMetadata, error) {
	return VaultMetadata{}, nil
}

type recordingSQLExecutor struct {
	mu          sync.Mutex
	credentials []string
}

func (e *recordingSQLExecutor) Open(
	_ context.Context, _ SQLConfig, credential Credential,
) (SQLSession, error) {
	e.mu.Lock()
	e.credentials = append(e.credentials, credential.Username()+"\x00"+credential.Password())
	e.mu.Unlock()
	return &recordingSQLSession{}, nil
}

func (e *recordingSQLExecutor) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.credentials...)
}

type recordingSQLSession struct{}

func (*recordingSQLSession) Describe(context.Context, string) (int, error) { return 2, nil }

func (*recordingSQLSession) Query(
	_ context.Context, _ string, _ []any, sink SQLSink,
) error {
	if err := sink.Columns([]string{"id", "total"}); err != nil {
		return err
	}
	return sink.Row(json.RawMessage(`[1,"9.90"]`))
}

func (*recordingSQLSession) Close(context.Context) error { return nil }

type failedSQLRunner struct{}

func (failedSQLRunner) RunBound(
	context.Context, string, string, string, domain.Scope, map[string]any,
) (SQLResult, error) {
	return SQLResult{
		Columns: []string{usernameCanary}, Rows: []json.RawMessage{json.RawMessage(`["partial-row"]`)},
		IssuanceOutcome: IssuanceSucceeded,
		Issuance:        Issuance{SQLInstance: "app-x", VaultInstance: "prod", Role: "readonly"},
		Revocation:      RevocationFailed,
	}, fmt.Errorf("driver repeated %s", credentialCanary)
}
