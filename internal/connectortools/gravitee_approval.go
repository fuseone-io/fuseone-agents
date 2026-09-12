package connectortools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
)

const graviteeContractVersion = "gravitee-contract/v1"

// RequiresApprovalEvidence names native writes whose approval is meaningful
// only beside the server-owned evidence sealed into the request.
func RequiresApprovalEvidence(id domain.ToolID) bool {
	connector, _, operation, ok := parseToolID(id)
	return ok && connector == "gravitee" && operation == "accept_subscription"
}

// graviteeContractDigest binds every server-owned choice behind one approval.
// The credential bytes are deliberately absent; their configured source and
// the Vault endpoint are the authority the decision relies on.
func graviteeContractDigest(cfg GraviteeConfig, vault VaultConfig, operation string) (string, bool) {
	if operation != "gravitee.accept_subscription" || len(cfg.AllowedReferences) != 1 {
		return "", false
	}
	contract := struct {
		Version          string                   `json:"version"`
		Operation        string                   `json:"operation"`
		Address          string                   `json:"address"`
		Organization     string                   `json:"organization"`
		Environment      string                   `json:"environment"`
		Reference        GraviteeReference        `json:"reference"`
		MinTTLSeconds    int                      `json:"min_ttl_seconds"`
		MaxTTLSeconds    int                      `json:"max_ttl_seconds"`
		AllowNoExpiry    bool                     `json:"allow_no_expiry"`
		CredentialSource GraviteeCredentialSource `json:"credential_source"`
		VaultAddress     string                   `json:"vault_address"`
		VaultNamespace   string                   `json:"vault_namespace"`
	}{
		Version: graviteeContractVersion, Operation: operation,
		Address: cfg.Address, Organization: cfg.Organization, Environment: cfg.Environment,
		Reference: cfg.AllowedReferences[0], MinTTLSeconds: cfg.MinTTLSeconds,
		MaxTTLSeconds: cfg.MaxTTLSeconds, AllowNoExpiry: cfg.AllowNoExpiry,
		CredentialSource: cfg.CredentialSource,
		VaultAddress:     vault.Address, VaultNamespace: vault.Namespace,
	}
	body, err := json.Marshal(contract)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(body)
	return graviteeContractVersion + ":sha256:" + hex.EncodeToString(sum[:]), true
}
