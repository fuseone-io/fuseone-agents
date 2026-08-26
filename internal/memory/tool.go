package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const (
	CodeMemoryArgumentsInvalid = "memory_arguments_invalid"
	CodeMemoryLearningDisabled = "memory_learning_disabled"
	CodeMemoryUnavailable      = "memory_unavailable"
)

const memoryToolDescription = "Find governed structured memory assertions for this agent and scope."
const memorySuggestDescription = "Suggest a structured memory assertion for review or automatic confirmation."

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

type findArgs struct {
	Kind      string `json:"kind,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Signature string `json:"signature,omitempty"`
	Search    string `json:"search,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type suggestArgs struct {
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	Signature string `json:"signature"`
	Claim     string `json:"claim"`
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
	body, labels, stats, err := memoryResult(found)
	returned, omitted = stats.Returned, stats.Omitted
	if err != nil {
		return engine.ToolResult{}, err
	}
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, body)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("memory: store result: %w", err)
	}
	succeeded = true
	return engine.ToolResult{ResultRef: ref, Labels: labels}, nil
}

func (l *Layer) suggest(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	args, valid := decodeSuggestArgs(call.Args)
	if !valid || call.AgentID == "" || l.store == nil || l.content == nil {
		return failed(CodeMemoryArgumentsInvalid), nil
	}
	policy := call.MemoryLearning.Normalize()
	if !policy.Enabled() {
		return failed(CodeMemoryLearningDisabled), nil
	}
	now := nowOrWall(call.At)
	suggestion := domain.MemorySuggestion{
		Scope: call.Scope, AgentID: call.AgentID,
		Kind: args.Kind, Subject: args.Subject, Signature: args.Signature,
		Claim: args.Claim, Labels: call.Labels.Clone(),
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
	return engine.ToolResult{ResultRef: ref}, nil
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

func decodeSuggestArgs(raw []byte) (suggestArgs, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var args suggestArgs
	if err := dec.Decode(&args); err != nil {
		return suggestArgs{}, false
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

type memoryResultPayload struct {
	Assertions    []resultAssertion `json:"assertions"`
	Omitted       int               `json:"omitted,omitempty"`
	OmittedReason string            `json:"omitted_reason,omitempty"`
	ByteBudget    int               `json:"byte_budget,omitempty"`
}

type memoryResultStats struct {
	Returned int
	Omitted  int
}

type memorySuggestPayload struct {
	Status       string `json:"status"`
	SuggestionID string `json:"suggestion_id,omitempty"`
	AssertionID  string `json:"assertion_id,omitempty"`
	Observations int64  `json:"observations,omitempty"`
	Confirmed    int64  `json:"confirmed,omitempty"`
}

const maxMemoryResultBytes = 16 * 1024

func memoryResult(found []domain.MemoryAssertion) ([]byte, domain.Labels, memoryResultStats, error) {
	assertions := make([]resultAssertion, 0, len(found))
	var labels domain.Labels
	for _, a := range found {
		labels = labels.Union(a.Labels)
	}
	for _, a := range found {
		next := append(assertions, resultAssertionFrom(a))
		body, err := marshalMemoryResult(next, len(found))
		if err != nil {
			return nil, domain.Labels{}, memoryResultStats{}, err
		}
		if len(body) > maxMemoryResultBytes {
			if len(assertions) == 0 {
				body, err := marshalMemoryResult(nil, len(found))
				return body, labels, memoryResultStats{Omitted: len(found)}, err
			}
			break
		}
		assertions = next
	}
	body, err := marshalMemoryResult(assertions, len(found))
	stats := memoryResultStats{Returned: len(assertions), Omitted: len(found) - len(assertions)}
	return body, labels, stats, err
}

func resultAssertionFrom(a domain.MemoryAssertion) resultAssertion {
	expires := ""
	var expiresAt *string
	if a.ExpiresAt != nil {
		expires = a.ExpiresAt.UTC().Format(timeFormat)
		expiresAt = &expires
	}
	return resultAssertion{
		ID: a.ID, Kind: a.Kind, Subject: a.Subject,
		Signature: a.Signature, Claim: a.Claim,
		Evidence:     slices.Clone(a.Evidence),
		Observations: a.Observations, Confirmed: a.Confirmed,
		ExpiresAt: expiresAt, UpdatedAt: a.UpdatedAt.UTC().Format(timeFormat),
	}
}

func marshalMemoryResult(assertions []resultAssertion, total int) ([]byte, error) {
	if assertions == nil {
		assertions = []resultAssertion{}
	}
	out := memoryResultPayload{Assertions: assertions}
	if omitted := total - len(assertions); omitted > 0 {
		out.Omitted = omitted
		out.OmittedReason = "result_byte_budget"
		out.ByteBudget = maxMemoryResultBytes
	}
	return json.Marshal(out)
}

func memorySuggestResult(out domain.MemorySuggestionOutcome) memorySuggestPayload {
	payload := memorySuggestPayload{
		Status:       string(out.Result),
		SuggestionID: out.Suggestion.ID,
		AssertionID:  out.Suggestion.AssertionID,
		Observations: out.Suggestion.Observations,
	}
	if out.Assertion != nil {
		payload.AssertionID = out.Assertion.ID
		payload.Confirmed = out.Assertion.Confirmed
	}
	return payload
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

func memorySuggestSchema() map[string]any {
	text := func(description string, max int) map[string]any {
		return map[string]any{"type": "string", "maxLength": max, "description": description}
	}
	return map[string]any{
		"type":                 "object",
		"required":             []string{"kind", "subject", "signature", "claim"},
		"additionalProperties": false,
		"properties": map[string]any{
			"kind":      text("Stable assertion kind.", domain.MaxMemoryKindBytes),
			"subject":   text("Thing this assertion is about.", domain.MaxMemorySubjectBytes),
			"signature": text("Stable key for the repeated situation.", domain.MaxMemorySignatureBytes),
			"claim":     text("Small falsifiable claim to remember.", domain.MaxMemoryClaimBytes),
		},
	}
}

func failed(code string) engine.ToolResult {
	return engine.ToolResult{Failed: true, ErrorCode: code}
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
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
