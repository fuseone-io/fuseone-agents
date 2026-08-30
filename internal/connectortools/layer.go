package connectortools

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// Layer adds governed connector tools beside an existing tool layer.
type Layer struct {
	base    engine.Tools
	catalog engine.Catalog
	content engine.ContentStore
	vault   VaultClient
	sql     SQLRunner

	mu        sync.RWMutex
	instances map[instanceKey]Instance
}

// SQLRunner executes one registered template under authority resolved by the
// platform. Kept smaller than SQLRuntime so the native boundary can be tested
// without a database or a Vault server.
type SQLRunner interface {
	RunBound(
		ctx context.Context, instance, templateID, contractDigest string,
		scope domain.Scope, params map[string]any,
	) (SQLResult, error)
}

type instanceKey struct {
	connector string
	name      string
}

func New(base engine.Tools, catalog engine.Catalog, content engine.ContentStore, vault VaultClient) *Layer {
	return &Layer{
		base: base, catalog: catalog, content: content, vault: vault,
		instances: map[instanceKey]Instance{},
	}
}

// WithSQLRuntime enables the SQL native path. Without it, a configured SQL
// instance remains unavailable rather than falling through to an MCP server.
func (l *Layer) WithSQLRuntime(sql SQLRunner) *Layer {
	l.sql = sql
	return l
}

// WithSQLMetrics attaches process metrics after the worker registry exists.
// A test runner or another SQLRunner has no metrics hook to configure, which
// keeps the execution port itself small.
func (l *Layer) WithSQLMetrics(metrics SQLRuntimeMetrics) *Layer {
	if runtime, ok := l.sql.(*SQLRuntime); ok {
		runtime.WithMetrics(metrics)
	}
	return l
}

func (l *Layer) SetInstances(instances []Instance) error {
	next := make(map[instanceKey]Instance, len(instances))
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		if _, err := instance.ToolID("noop"); err != nil {
			return err
		}
		key := instanceKey{connector: instance.Connector, name: instance.Name}
		next[key] = cloneInstance(instance)
	}
	l.mu.Lock()
	l.instances = next
	l.mu.Unlock()
	return nil
}

func (l *Layer) Effect(id domain.ToolID) (domain.Effect, bool) {
	if native, op, ok := l.native(id); ok {
		return operationEffect(op), native.Enabled
	}
	if l.catalog == nil {
		return domain.EffectUnknown, false
	}
	return l.catalog.Effect(id)
}

func (l *Layer) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	if _, _, ok := l.native(id); ok {
		return domain.ToolDedupe{}, false
	}
	if l.catalog == nil {
		return domain.ToolDedupe{}, false
	}
	return l.catalog.Dedupe(id)
}

func (l *Layer) ApprovalBinding(call engine.Call) string {
	instance, op, ok := l.native(call.Tool)
	if !ok {
		if binder, ok := l.base.(engine.ApprovalBinder); ok {
			return binder.ApprovalBinding(call)
		}
		return ""
	}
	if instance.Connector != "sql" || op.ID != "sql.run_query_template" {
		return ""
	}
	args, ok := decodeSQLRunArgs(call.Args)
	if !ok {
		return ""
	}
	vault, ok := l.sqlVaultEndpoint(instance)
	if !ok {
		return ""
	}
	digest, _ := sqlContractDigest(instance.SQL, vault, args.TemplateID)
	return digest
}

func (l *Layer) sqlVaultEndpoint(sql Instance) (VaultConfig, bool) {
	key := instanceKey{connector: "vault", name: sql.SQL.CredentialSource.VaultInstance}
	l.mu.RLock()
	vault, found := l.instances[key]
	l.mu.RUnlock()
	if !found || !vault.Enabled || !vault.Scope.Contains(sql.Scope) || vault.Vault.Address == "" {
		return VaultConfig{}, false
	}
	return vault.Vault, true
}

func (l *Layer) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	instance, op, ok := l.native(id)
	if ok {
		schema, ok := schemaFor(op.ID)
		return string(id), op.Summary, schema, ok && instance.Enabled
	}
	if schemas, ok := l.catalog.(interface {
		Schema(domain.ToolID) (string, string, map[string]any, bool)
	}); ok {
		return schemas.Schema(id)
	}
	return "", "", nil, false
}

func (l *Layer) Reserve(ctx context.Context, call engine.Call) error {
	instance, op, ok := l.native(call.Tool)
	if !ok {
		if l.base == nil {
			return fmt.Errorf("connector: no tool layer for %s", call.Tool)
		}
		return l.base.Reserve(ctx, call)
	}
	if !instance.Enabled {
		return fmt.Errorf("%w: %s", ErrUnavailable, call.Tool)
	}
	if !instance.Scope.Contains(call.Scope) {
		return fmt.Errorf("%w: %s in %s", ErrOutOfScope, call.Tool, call.Scope)
	}
	if instance.Connector == "sql" && op.ID == "sql.run_query_template" {
		current := l.ApprovalBinding(call)
		if current != call.ContractDigest && (current != "" || call.ContractDigest != "") {
			return ErrSQLContractChanged
		}
	}
	return nil
}

func (l *Layer) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	instance, op, ok := l.native(call.Tool)
	if !ok {
		if l.base == nil {
			return engine.ToolResult{}, fmt.Errorf("connector: no tool layer for %s", call.Tool)
		}
		return l.base.Invoke(ctx, call)
	}
	switch instance.Connector {
	case "vault":
		return l.invokeVaultNative(ctx, instance, op, call)
	case "sql":
		return l.invokeSQLNative(ctx, instance, op, call)
	default:
		return failed(CodeConnectorUnavailable), nil
	}
}

func (l *Layer) native(id domain.ToolID) (Instance, connectors.Operation, bool) {
	connector, name, operation, ok := parseToolID(id)
	if !ok {
		return Instance{}, connectors.Operation{}, false
	}
	l.mu.RLock()
	instance, found := l.instances[instanceKey{connector: connector, name: name}]
	l.mu.RUnlock()
	if !found {
		return Instance{}, connectors.Operation{}, false
	}
	_, op, found := connectors.OperationByID(connector + "." + operation)
	return cloneInstance(instance), op, found
}

func cloneInstance(in Instance) Instance {
	out := in
	out.Vault.AllowedPathPrefixes = slices.Clone(in.Vault.AllowedPathPrefixes)
	out.SQL.Templates = slices.Clone(in.SQL.Templates)
	for i := range out.SQL.Templates {
		out.SQL.Templates[i].Parameters = slices.Clone(in.SQL.Templates[i].Parameters)
	}
	return out
}
