package connectortools

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

var vaultFieldRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func (l *Layer) resolveVaultFields(
	ctx context.Context, call engine.Call, requested map[string]vaultFieldValue,
) (map[string]VaultSecretField, domain.Labels, string) {
	if l.content == nil {
		return nil, nil, CodeConnectorArtifactMissing
	}
	fields := make(map[string]VaultSecretField, len(requested))
	var labels domain.Labels
	for name, source := range requested {
		if !vaultFieldRE.MatchString(name) {
			return nil, nil, CodeConnectorBadArguments
		}
		body, artifact, code := l.artifactBody(ctx, call.ContextArtifacts, source.Artifact)
		if code != "" {
			return nil, nil, code
		}
		field, ok := vaultSecretField(body, source.Encoding)
		if !ok {
			return nil, nil, CodeConnectorBadArguments
		}
		fields[name] = field
		labels = labels.Union(artifact.Labels)
	}
	return fields, labels, ""
}

func (l *Layer) artifactBody(
	ctx context.Context, artifacts []domain.ContextArtifact, name string,
) ([]byte, domain.ContextArtifact, string) {
	artifact, ok := artifactNamed(artifacts, name)
	if !ok {
		return nil, domain.ContextArtifact{}, CodeConnectorArtifactMissing
	}
	body, err := l.content.Get(ctx, artifact.Ref)
	if err != nil {
		return nil, domain.ContextArtifact{}, CodeConnectorArtifactMissing
	}
	if digest(body) != artifact.Digest {
		return nil, domain.ContextArtifact{}, CodeConnectorDigestMismatch
	}
	return body, artifact, ""
}

func artifactNamed(artifacts []domain.ContextArtifact, name string) (domain.ContextArtifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Name == name && artifact.Ref != "" && artifact.Digest != "" {
			return artifact, true
		}
	}
	return domain.ContextArtifact{}, false
}

func (l *Layer) storeJSON(
	ctx context.Context, call engine.Call, labels domain.Labels, body any,
) (engine.ToolResult, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("connector: encode result: %w", err)
	}
	ref, err := l.content.Put(ctx, call.RunID, call.Seq, raw)
	if err != nil {
		return engine.ToolResult{}, fmt.Errorf("connector: store result: %w", err)
	}
	return engine.ToolResult{
		ResultRef: ref, ResultDigest: engine.ResultDigest(raw), ResultBytes: int64(len(raw)),
		Labels: labels,
	}, nil
}

func vaultWriteResponse(path string, fields map[string]vaultFieldValue, written VaultWriteResult) map[string]any {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	return map[string]any{
		"operation": "vault.write_secret",
		"path":      path,
		"fields":    names,
		"version":   written.Version,
	}
}

func vaultMetadataResponse(path string, metadata VaultMetadata) map[string]any {
	return map[string]any{
		"operation":       "vault.read_metadata",
		"path":            path,
		"current_version": metadata.CurrentVersion,
		"versions":        metadata.Versions,
		"keys":            metadata.Keys,
	}
}

func allowedPath(prefixes []string, raw string) (string, bool) {
	cleaned := cleanVaultPath(raw)
	if cleaned == "" {
		return "", false
	}
	for _, prefix := range prefixes {
		allowed := cleanVaultPath(prefix)
		if allowed == "" {
			continue
		}
		if cleaned == allowed || strings.HasPrefix(cleaned, allowed+"/") {
			return cleaned, true
		}
	}
	return "", false
}

func cleanVaultPath(raw string) string {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" || strings.Contains(trimmed, "\x00") {
		return ""
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+trimmed), "/")
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return ""
	}
	return cleaned
}

func failed(code string) engine.ToolResult {
	return engine.ToolResult{Failed: true, ErrorCode: code}
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

func vaultSecretField(body []byte, encoding string) (VaultSecretField, bool) {
	switch strings.TrimSpace(encoding) {
	case "", "text":
		if !utf8.Valid(body) {
			return VaultSecretField{}, false
		}
		return VaultSecretField{Value: body, Encoding: "text"}, true
	case "base64":
		encoded := base64.StdEncoding.EncodeToString(body)
		return VaultSecretField{Value: []byte(encoded), Encoding: "base64"}, true
	default:
		return VaultSecretField{}, false
	}
}
