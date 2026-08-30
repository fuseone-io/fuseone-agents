package connectortools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/engine"
)

type VaultClient interface {
	WriteSecret(ctx context.Context, cfg VaultConfig, token, secretPath string, fields map[string]VaultSecretField) (VaultWriteResult, error)
	ReadMetadata(ctx context.Context, cfg VaultConfig, token, secretPath string) (VaultMetadata, error)
	RevokeLease(ctx context.Context, cfg VaultConfig, token, leaseID string) error
}

type VaultSecretField struct {
	Value    []byte
	Encoding string
}

type VaultWriteResult struct {
	Version int `json:"version,omitempty"`
}

type VaultMetadata struct {
	CurrentVersion int      `json:"current_version,omitempty"`
	Versions       []int    `json:"versions,omitempty"`
	Keys           []string `json:"keys,omitempty"`
}

func (l *Layer) invokeNative(
	ctx context.Context, instance Instance, op connectors.Operation, call engine.Call,
) (engine.ToolResult, error) {
	if instance.Connector != "vault" {
		return failed(CodeConnectorUnavailable), nil
	}
	if l.vault == nil || instance.Token == "" {
		return failed(CodeConnectorUnavailable), nil
	}
	switch op.ID {
	case "vault.write_secret":
		return l.vaultWrite(ctx, instance, call)
	case "vault.read_metadata":
		return l.vaultReadMetadata(ctx, instance, call)
	case "vault.revoke_lease":
		return l.vaultRevokeLease(ctx, instance, call)
	default:
		return failed(CodeConnectorUnavailable), nil
	}
}

type vaultWriteArgs struct {
	Path   string                     `json:"path"`
	Fields map[string]vaultFieldValue `json:"fields"`
}

type vaultFieldValue struct {
	Artifact string `json:"artifact"`
	Encoding string `json:"encoding,omitempty"`
}

func (l *Layer) vaultWrite(ctx context.Context, instance Instance, call engine.Call) (engine.ToolResult, error) {
	var args vaultWriteArgs
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return failed(CodeConnectorBadArguments), nil
	}
	secretPath, ok := allowedPath(instance.Vault.AllowedPathPrefixes, args.Path)
	if !ok || len(args.Fields) == 0 {
		return failed(CodeConnectorPathNotAllowed), nil
	}
	fields, labels, errCode := l.resolveVaultFields(ctx, call, args.Fields)
	if errCode != "" {
		return failed(errCode), nil
	}
	written, err := l.vault.WriteSecret(ctx, instance.Vault, instance.Token, secretPath, fields)
	if err != nil {
		return failed(CodeConnectorUpstreamFailed), nil
	}
	return l.storeJSON(ctx, call, labels, vaultWriteResponse(secretPath, args.Fields, written))
}

func (l *Layer) vaultReadMetadata(ctx context.Context, instance Instance, call engine.Call) (engine.ToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return failed(CodeConnectorBadArguments), nil
	}
	secretPath, ok := allowedPath(instance.Vault.AllowedPathPrefixes, args.Path)
	if !ok {
		return failed(CodeConnectorPathNotAllowed), nil
	}
	metadata, err := l.vault.ReadMetadata(ctx, instance.Vault, instance.Token, secretPath)
	if err != nil {
		return failed(CodeConnectorUpstreamFailed), nil
	}
	return l.storeJSON(ctx, call, nil, vaultMetadataResponse(secretPath, metadata))
}

func (l *Layer) vaultRevokeLease(ctx context.Context, instance Instance, call engine.Call) (engine.ToolResult, error) {
	var args struct {
		LeaseID string `json:"lease_id"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return failed(CodeConnectorBadArguments), nil
	}
	if strings.TrimSpace(args.LeaseID) == "" {
		return failed(CodeConnectorBadArguments), nil
	}
	if err := l.vault.RevokeLease(ctx, instance.Vault, instance.Token, args.LeaseID); err != nil {
		return failed(CodeConnectorUpstreamFailed), nil
	}
	return l.storeJSON(ctx, call, nil, map[string]any{
		"operation": "vault.revoke_lease",
		"revoked":   true,
	})
}

/*
VaultDatabaseCredential is what the database secrets engine answers.

Kept apart from the operations a model can reach: this is issued by the SQL
runtime for one approved query, and there is no tool that returns it. The
type carries the raw fields because it is the wire shape; the moment it is
resolved it becomes a Credential, which cannot be printed.
*/
type VaultDatabaseCredential struct {
	Username        string
	Password        string
	LeaseID         string
	LeaseTTLSeconds int
}

// valid refuses a partial answer. A credential without a lease id cannot be
// revoked, and one without a TTL cannot be bounded — so an incomplete response
// is a refusal rather than something to use until it stops working.
func (c VaultDatabaseCredential) valid() error {
	switch {
	case c.Username == "" || c.Password == "":
		return ErrCredentialIncomplete
	case c.LeaseID == "":
		return ErrCredentialIncomplete
	case c.LeaseTTLSeconds <= 0:
		return ErrCredentialIncomplete
	}
	return nil
}
