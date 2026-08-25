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

	mu        sync.RWMutex
	instances map[instanceKey]Instance
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
	instance, _, ok := l.native(call.Tool)
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
	return l.invokeNative(ctx, instance, op, call)
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
	return out
}
