package connectortools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const sqlContractVersion = "sql-contract/v2"

// sqlContractDigest binds everything the server chooses for one template.
// Parameters supplied by the model are bound separately by ArgsDigest.
func sqlContractDigest(cfg SQLConfig, vault VaultConfig, templateID string) (string, bool) {
	tpl, ok := cfg.Template(templateID)
	if !ok {
		return "", false
	}
	contract := struct {
		Version          string           `json:"version"`
		Driver           SQLDriver        `json:"driver"`
		Host             string           `json:"host"`
		Port             int              `json:"port"`
		Database         string           `json:"database"`
		CredentialSource CredentialSource `json:"credential_source"`
		VaultAddress     string           `json:"vault_address"`
		VaultNamespace   string           `json:"vault_namespace"`
		Template         SQLTemplate      `json:"template"`
	}{
		Version: sqlContractVersion, Driver: cfg.Driver, Host: cfg.Host,
		Port: cfg.Port, Database: cfg.Database,
		CredentialSource: cfg.CredentialSource,
		VaultAddress:     vault.Address, VaultNamespace: vault.Namespace,
		Template: tpl,
	}
	body, err := json.Marshal(contract)
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(body)
	return sqlContractVersion + ":sha256:" + hex.EncodeToString(sum[:]), true
}
