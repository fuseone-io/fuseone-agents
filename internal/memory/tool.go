package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const (
	CodeMemoryArgumentsInvalid = "memory_arguments_invalid"
	CodeMemoryUnavailable      = "memory_unavailable"
)

const memoryToolDescription = "Find governed structured memory assertions for this agent and scope."

type ToolSource interface {
	Tools(ctx context.Context) ([]domain.ToolEntry, error)
}

type ToolList struct{ base ToolSource }

func NewToolList(base ToolSource) *ToolList { return &ToolList{base: base} }

func (l *ToolList) Tools(ctx context.Context) ([]domain.ToolEntry, error) {
	var out []domain.ToolEntry
	if l != nil && l.base != nil {
		base, err := l.base.Tools(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, base...)
	}
	return sortedToolEntries(append(out, MemoryToolEntry())), nil
}

func MemoryToolEntry() domain.ToolEntry {
	return domain.ToolEntry{
		ID: domain.ToolMemoryFind, Server: "fuseone:memory",
		Description: memoryToolDescription,
		Effect:      domain.EffectRead,
		Untrusted:   true,
		Native:      true,
		Scope:       domain.Scope{Company: domain.Installation},
		OnSurface:   true,
	}
}

// Layer adds the platform-owned memory reader beside an existing tool layer.
type Layer struct {
	base    engine.Tools
	catalog engine.Catalog
	content engine.ContentStore
	store   Store
}

func NewLayer(base engine.Tools, catalog engine.Catalog, content engine.ContentStore, store Store) *Layer {
	return &Layer{base: base, catalog: catalog, content: content, store: store}
}

func (l *Layer) Effect(id domain.ToolID) (domain.Effect, bool) {
	if id == domain.ToolMemoryFind {
		return domain.EffectRead, true
	}
	if l.catalog == nil {
		return domain.EffectUnknown, false
	}
	return l.catalog.Effect(id)
}

func (l *Layer) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	if id == domain.ToolMemoryFind || l.catalog == nil {
		return domain.ToolDedupe{}, false
	}
	return l.catalog.Dedupe(id)
}

func (l *Layer) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if id == domain.ToolMemoryFind {
		return string(id), memoryToolDescription, memoryFindSchema(), true
	}
	if schemas, ok := l.catalog.(interface {
		Schema(domain.ToolID) (string, string, map[string]any, bool)
	}); ok {
		return schemas.Schema(id)
	}
	return "", "", nil, false
}

func (l *Layer) Reserve(ctx context.Context, call engine.Call) error {
	if call.Tool == domain.ToolMemoryFind {
		return nil
	}
	if l.base == nil {
		return fmt.Errorf("memory: no tool layer for %s", call.Tool)
	}
	return l.base.Reserve(ctx, call)
}

func (l *Layer) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	if call.Tool != domain.ToolMemoryFind {
		if l.base == nil {
			return engine.ToolResult{}, fmt.Errorf("memory: no tool layer for %s", call.Tool)
		}
		return l.base.Invoke(ctx, call)
	}
	return l.find(ctx, call)
}

type findArgs struct {
	Kind      string `json:"kind,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Signature string `json:"signature,omitempty"`
	Search    string `json:"search,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

func (l *Layer) find(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	args, ok := decodeFindArgs(call.Args)
	if !ok || call.AgentID == "" || l.store == nil || l.content == nil {
		return failed(CodeMemoryArgumentsInvalid), nil
	}
	found, err := l.store.Find(ctx, domain.MemoryQuery{
		Scope: call.Scope, AgentID: call.AgentID, Kind: args.Kind,
		Subject: args.Subject, Signature: args.Signature,
		Search: args.Search, Limit: args.Limit,
	})
	if err != nil {
		return failed(CodeMemoryUnavailable), nil
	}
	body, labels, err := memoryResult(found)
	if err != nil {
		return engine.ToolResult{}, err
	}
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, body)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("memory: store result: %w", err)
	}
	return engine.ToolResult{ResultRef: ref, Labels: labels}, nil
}

func decodeFindArgs(raw []byte) (findArgs, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return findArgs{}, true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var args findArgs
	if err := dec.Decode(&args); err != nil {
		return findArgs{}, false
	}
	return args, dec.Decode(&struct{}{}) == io.EOF
}

type resultAssertion struct {
	ID           string                  `json:"id"`
	Kind         string                  `json:"kind"`
	Subject      string                  `json:"subject"`
	Signature    string                  `json:"signature"`
	Claim        string                  `json:"claim"`
	Evidence     []domain.MemoryEvidence `json:"evidence"`
	Observations int64                   `json:"observations,omitempty"`
	Confirmed    int64                   `json:"confirmed,omitempty"`
	ExpiresAt    *string                 `json:"expires_at,omitempty"`
	UpdatedAt    string                  `json:"updated_at"`
}

func memoryResult(found []domain.MemoryAssertion) ([]byte, domain.Labels, error) {
	out := struct {
		Assertions []resultAssertion `json:"assertions"`
	}{Assertions: make([]resultAssertion, 0, len(found))}
	var labels domain.Labels
	for _, a := range found {
		expires := ""
		var expiresAt *string
		if a.ExpiresAt != nil {
			expires = a.ExpiresAt.UTC().Format(timeFormat)
			expiresAt = &expires
		}
		out.Assertions = append(out.Assertions, resultAssertion{
			ID: a.ID, Kind: a.Kind, Subject: a.Subject,
			Signature: a.Signature, Claim: a.Claim,
			Evidence:     slices.Clone(a.Evidence),
			Observations: a.Observations, Confirmed: a.Confirmed,
			ExpiresAt: expiresAt, UpdatedAt: a.UpdatedAt.UTC().Format(timeFormat),
		})
		labels = labels.Union(a.Labels)
	}
	body, err := json.Marshal(out)
	return body, labels, err
}

const timeFormat = "2006-01-02T15:04:05Z"

func memoryFindSchema() map[string]any {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return map[string]any{
		"kind":      text("Optional assertion kind to match exactly."),
		"subject":   text("Optional subject to match exactly."),
		"signature": text("Optional signature to match exactly."),
		"search":    text("Optional text to search in subject, signature and claim."),
		"limit": map[string]any{
			"type": "integer", "minimum": 1, "maximum": domain.MaxMemoryFindLimit,
			"description": "Maximum assertions to return.",
		},
	}
}

func failed(code string) engine.ToolResult {
	return engine.ToolResult{Failed: true, ErrorCode: code}
}

func sortedToolEntries(in []domain.ToolEntry) []domain.ToolEntry {
	byID := map[domain.ToolID]domain.ToolEntry{}
	for _, entry := range in {
		byID[entry.ID] = entry
	}
	out := make([]domain.ToolEntry, 0, len(byID))
	for _, entry := range byID {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return out
}
