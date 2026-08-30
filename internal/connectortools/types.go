// Package connectortools exposes FuseOne-owned governed connectors as tools.
//
// These are native operations, not MCP. They still cross the same engine.Tools
// and engine.Catalog ports, so the Gate, ledger, content store and retention
// rules see them exactly like other tool calls.
package connectortools

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

const (
	CodeConnectorUnavailable     = "connector_unavailable"
	CodeConnectorOutOfScope      = "connector_out_of_scope"
	CodeConnectorBadArguments    = "connector_bad_arguments"
	CodeConnectorPathNotAllowed  = "connector_path_not_allowed"
	CodeConnectorArtifactMissing = "connector_artifact_missing"
	CodeConnectorDigestMismatch  = "connector_digest_mismatch"
	CodeConnectorUpstreamFailed  = "connector_upstream_failed"
)

var instanceNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func ValidInstanceName(name string) bool { return instanceNameRE.MatchString(name) }

// Instance is one configured connector endpoint.
type Instance struct {
	Connector string
	Name      string
	Scope     domain.Scope
	Enabled   bool

	Vault VaultConfig
	SQL   SQLConfig
	// Token is what the instance authenticates with, for the connectors that
	// authenticate with one. RequiresToken says which; SQL is not among them.
	Token string
	// HasToken says a token is stored without revealing it. Configuration read
	// back from settings never carries the bytes, so a check that asked Token
	// would call every real vault tokenless while accepting an in-memory
	// fixture. Presence is metadata; the value is not.
	HasToken bool
}

// TokenPresent is whether this instance can authenticate: the value in hand,
// or a stored one the reader was not given.
func (i Instance) TokenPresent() bool {
	return i.HasToken || strings.TrimSpace(i.Token) != ""
}

// VaultConfig is the non-secret Vault configuration.
type VaultConfig struct {
	Address             string
	Mount               string
	Namespace           string
	AllowedPathPrefixes []string
}

func (i Instance) ToolID(operation string) (domain.ToolID, error) {
	if !ValidInstanceName(i.Name) {
		return "", fmt.Errorf("connector: invalid instance name %q", i.Name)
	}
	operation = strings.TrimPrefix(operation, i.Connector+".")
	if operation == "" || strings.Contains(operation, ".") {
		return "", fmt.Errorf("connector: invalid operation %q", operation)
	}
	return domain.ToolID(i.Connector + "." + i.Name + "." + operation), nil
}

func parseToolID(id domain.ToolID) (connector, instance, operation string, ok bool) {
	parts := strings.Split(string(id), ".")
	if len(parts) != 3 {
		return "", "", "", false
	}
	if parts[0] == "" || !ValidInstanceName(parts[1]) || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
