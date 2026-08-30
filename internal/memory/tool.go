package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// The two memory tools an agent may call, and the layer that answers them.
//
// A native tool rather than something the model was told about in a prompt:
// what it may read is bounded by the run's scope and agent, and what it writes
// is a proposal nobody has agreed to yet. Neither is negotiable from inside the
// conversation.

const (
	CodeMemoryArgumentsInvalid = "memory_arguments_invalid"
	CodeMemoryLearningDisabled = "memory_learning_disabled"
	CodeMemoryUnavailable      = "memory_unavailable"
)

const memoryToolDescription = "Find governed structured memory assertions for this agent and scope."

const memorySuggestDescription = "Suggest a fact for review or automatic confirmation; the platform derives its identity from the subject."

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
	return sortedToolEntries(append(out, MemoryToolEntry(), MemorySuggestToolEntry())), nil
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

func MemorySuggestToolEntry() domain.ToolEntry {
	return domain.ToolEntry{
		ID: domain.ToolMemorySuggest, Server: "fuseone:memory",
		Description: memorySuggestDescription,
		Effect:      domain.EffectWrite,
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
	metrics Metrics
}

func NewLayer(base engine.Tools, catalog engine.Catalog, content engine.ContentStore, store Store) *Layer {
	return &Layer{base: base, catalog: catalog, content: content, store: store}
}

type Metrics interface {
	MemoryFind(duration time.Duration, returned int, omitted int, failed bool)
}

func (l *Layer) WithMetrics(metrics Metrics) *Layer {
	l.metrics = metrics
	return l
}

func (l *Layer) Effect(id domain.ToolID) (domain.Effect, bool) {
	if id == domain.ToolMemoryFind {
		return domain.EffectRead, true
	}
	if id == domain.ToolMemorySuggest {
		return domain.EffectWrite, true
	}
	if l.catalog == nil {
		return domain.EffectUnknown, false
	}
	return l.catalog.Effect(id)
}

func (l *Layer) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	if id == domain.ToolMemoryFind || id == domain.ToolMemorySuggest || l.catalog == nil {
		return domain.ToolDedupe{}, false
	}
	return l.catalog.Dedupe(id)
}

func (l *Layer) ApprovalBinding(call engine.Call) string {
	if call.Tool == domain.ToolMemoryFind || call.Tool == domain.ToolMemorySuggest {
		return ""
	}
	if binder, ok := l.base.(engine.ApprovalBinder); ok {
		return binder.ApprovalBinding(call)
	}
	return ""
}

func (l *Layer) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if id == domain.ToolMemoryFind {
		return string(id), memoryToolDescription, memoryFindSchema(), true
	}
	if id == domain.ToolMemorySuggest {
		return string(id), memorySuggestDescription, memorySuggestSchema(), true
	}
	if schemas, ok := l.catalog.(interface {
		Schema(domain.ToolID) (string, string, map[string]any, bool)
	}); ok {
		return schemas.Schema(id)
	}
	return "", "", nil, false
}

func (l *Layer) Reserve(ctx context.Context, call engine.Call) error {
	if call.Tool == domain.ToolMemoryFind || call.Tool == domain.ToolMemorySuggest {
		return nil
	}
	if l.base == nil {
		return fmt.Errorf("memory: no tool layer for %s", call.Tool)
	}
	return l.base.Reserve(ctx, call)
}

func (l *Layer) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	if call.Tool == domain.ToolMemorySuggest {
		return l.suggest(ctx, call)
	}
	if call.Tool != domain.ToolMemoryFind {
		if l.base == nil {
			return engine.ToolResult{}, fmt.Errorf("memory: no tool layer for %s", call.Tool)
		}
		return l.base.Invoke(ctx, call)
	}
	return l.find(ctx, call)
}

func (l *Layer) find(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	start := time.Now()
	var returned, omitted int
	succeeded := false
	defer func() {
		if l.metrics != nil {
			l.metrics.MemoryFind(time.Since(start), returned, omitted, !succeeded)
		}
	}()
	args, valid := decodeFindArgs(call.Args)
	if !valid || call.AgentID == "" || l.store == nil || l.content == nil {
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
	body, labels, stats, err := memoryResult(found, parseSearchTerms(args.Search))
	returned, omitted = stats.Returned, stats.Omitted
	if err != nil {
		return engine.ToolResult{}, err
	}
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, body)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("memory: store result: %w", err)
	}
	succeeded = true
	return engine.ToolResult{
		ResultRef: ref, ResultDigest: engine.ResultDigest(body), ResultBytes: int64(len(body)),
		Labels: labels,
	}, nil
}

func (l *Layer) suggest(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	args, valid := decodeSuggestArgs(call.Args)
	if !valid || call.AgentID == "" || l.store == nil || l.content == nil {
		return failed(CodeMemoryArgumentsInvalid), nil
	}
	policy := call.MemoryLearning.ForSuggestion(call.Labels)
	if !policy.Enabled() {
		return failed(CodeMemoryLearningDisabled), nil
	}
	now := nowOrWall(call.At)
	suggestion := domain.MemorySuggestion{
		Scope: call.Scope, AgentID: call.AgentID,
		Subject: args.Subject, Claim: args.Claim, Labels: call.Labels.Clone(),
		ExpiresAt: policy.ExpiresAt(now),
		Evidence: []domain.MemoryEvidence{{
			RunID: call.RunID, Artifact: domain.ArtifactMemorySuggestion,
			Digest: digestBytes(call.Args),
		}},
	}
	by := domain.UserID("agent:" + string(call.AgentID))
	if _, err := prepareSuggestion(suggestion, by, now); err != nil {
		return failed(CodeMemoryArgumentsInvalid), nil
	}
	out, err := l.store.Suggest(ctx, suggestion, policy, by, now)
	if err != nil {
		return failed(CodeMemoryUnavailable), nil
	}
	body, err := json.Marshal(memorySuggestResult(out))
	if err != nil {
		return engine.ToolResult{}, err
	}
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, body)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("memory: store suggestion result: %w", err)
	}
	return engine.ToolResult{
		ResultRef: ref, ResultDigest: engine.ResultDigest(body), ResultBytes: int64(len(body)),
	}, nil
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

func failed(code string) engine.ToolResult {
	return engine.ToolResult{Failed: true, ErrorCode: code}
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
