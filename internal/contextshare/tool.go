// Package contextshare implements FuseOne-owned context tools.
//
// These are not MCP tools. They read platform claim-checks that an event
// supplied to a run, while still crossing the same Gate and ledger path as an
// ordinary tool call.
package contextshare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const (
	CodeContextArtifactNotAllowed     = "context_artifact_not_allowed"
	CodeContextArtifactUnavailable    = "context_artifact_unavailable"
	CodeContextArtifactDigestMismatch = "context_artifact_digest_mismatch"
)

// Layer adds FuseOne context reads beside an existing tool catalogue.
type Layer struct {
	base    engine.Tools
	catalog engine.Catalog
	content engine.ContentStore
}

func New(base engine.Tools, catalog engine.Catalog, content engine.ContentStore) *Layer {
	return &Layer{base: base, catalog: catalog, content: content}
}

func (l *Layer) Effect(id domain.ToolID) (domain.Effect, bool) {
	if id == domain.ToolContextRead {
		return domain.EffectRead, true
	}
	if l.catalog == nil {
		return domain.EffectUnknown, false
	}
	return l.catalog.Effect(id)
}

func (l *Layer) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	if id == domain.ToolContextRead || l.catalog == nil {
		return domain.ToolDedupe{}, false
	}
	return l.catalog.Dedupe(id)
}

func (l *Layer) ApprovalBinding(call engine.Call) string {
	if call.Tool == domain.ToolContextRead {
		return ""
	}
	if binder, ok := l.base.(engine.ApprovalBinder); ok {
		return binder.ApprovalBinding(call)
	}
	return ""
}

func (l *Layer) Reserve(ctx context.Context, call engine.Call) error {
	if call.Tool == domain.ToolContextRead {
		return nil
	}
	if l.base == nil {
		return fmt.Errorf("context: no tool layer for %s", call.Tool)
	}
	return l.base.Reserve(ctx, call)
}

func (l *Layer) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	if call.Tool != domain.ToolContextRead {
		if l.base == nil {
			return engine.ToolResult{}, fmt.Errorf("context: no tool layer for %s", call.Tool)
		}
		return l.base.Invoke(ctx, call)
	}
	return l.read(ctx, call)
}

type readArgs struct {
	Name string `json:"name"`
}

func (l *Layer) read(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	var args readArgs
	_ = json.Unmarshal(call.Args, &args)
	name := strings.TrimSpace(args.Name)
	artifact, ok := artifactNamed(call.ContextArtifacts, name)
	if !ok {
		return failed(CodeContextArtifactNotAllowed), nil
	}
	if l.content == nil {
		return failed(CodeContextArtifactUnavailable), nil
	}

	body, err := l.content.Get(ctx, artifact.Ref)
	if err != nil {
		return failed(CodeContextArtifactUnavailable), nil
	}
	if got := digest(body); got != artifact.Digest {
		return failed(CodeContextArtifactDigestMismatch), nil
	}
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, body)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("context: store artifact %q: %w", artifact.Name, err)
	}

	contextArtifact := artifact
	return engine.ToolResult{
		ResultRef: ref, ResultDigest: engine.ResultDigest(body), ResultBytes: int64(len(body)),
		Labels: artifact.Labels.Clone(), Context: &contextArtifact,
	}, nil
}

func artifactNamed(artifacts []domain.ContextArtifact, name string) (domain.ContextArtifact, bool) {
	if name == "" {
		return domain.ContextArtifact{}, false
	}
	for _, artifact := range artifacts {
		if artifact.Name == name && artifact.Ref != "" && artifact.Digest != "" {
			return artifact, true
		}
	}
	return domain.ContextArtifact{}, false
}

func failed(code string) engine.ToolResult {
	return engine.ToolResult{Failed: true, ErrorCode: code}
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
