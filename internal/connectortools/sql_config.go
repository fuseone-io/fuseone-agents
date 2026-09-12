package connectortools

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

/*
Where a governed SQL instance gets its database authority.

The binding is fixed by an administrator; the credential is not. That is the
whole point of #52: no database password is stored in FuseOne, and the model
never sees the fields below — they are configuration, not tool schema.
*/
type CredentialSourceKind string

const (
	// CredentialVaultDatabaseRole issues a short-lived database credential
	// from a Vault database secrets engine role, just in time for one approved
	// operation.
	CredentialVaultDatabaseRole CredentialSourceKind = "vault_database_role"
)

type CredentialSource struct {
	Kind CredentialSourceKind
	// VaultInstance names a configured vault connector instance, resolved by
	// name against the instances that contain this one's scope.
	VaultInstance string
	Mount         string
	Role          string
}

/*
SQLConfig addresses a database without being able to spell a credential.

Host, port and database are separate fields rather than one connection string,
and that is a refusal rather than a style: a DSN is a single field that can
carry a password, and a validator that tries to recognise every driver's
spelling of one is a validator that falls behind. Refusing the shape is the
only version that stays true.
*/
type SQLConfig struct {
	Driver           SQLDriver
	Host             string
	Port             int
	Database         string
	CredentialSource CredentialSource
	Templates        []SQLTemplate
}

// RequiresToken is whether a connector authenticates with a token of its own.
//
// Asked per connector rather than assumed for every instance. Vault holds a
// token because it authenticates to Vault; bound connectors take authority
// from it and must not carry a second credential of their own.
func RequiresToken(connector string) bool { return connector == "vault" }

func validateSQLConfig(instance Instance) error {
	cfg := instance.SQL
	switch {
	case cfg.Driver != SQLDriverPostgres:
		return fmt.Errorf("connector: sql %s needs driver %q", instance.Name, SQLDriverPostgres)
	case strings.TrimSpace(cfg.Host) == "":
		return fmt.Errorf("connector: sql %s needs a host", instance.Name)
	case cfg.Port <= 0 || cfg.Port > 65535:
		return fmt.Errorf("connector: sql %s needs a port between 1 and 65535", instance.Name)
	case strings.TrimSpace(cfg.Database) == "":
		return fmt.Errorf("connector: sql %s needs a database", instance.Name)
	}
	if err := plainHost(instance.Name, cfg.Host); err != nil {
		return err
	}
	if err := validateCredentialSource(instance.Name, cfg.CredentialSource); err != nil {
		return err
	}
	return validateTemplates(instance.Name, instance.Enabled, cfg.Templates)
}

// plainHost refuses anything that is not just a host. A scheme, credentials,
// a port, a path or a query means somebody wrote an address that can carry
// more than a destination.
func plainHost(name, host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	if !hostname.MatchString(host) {
		return fmt.Errorf(
			"connector: sql %s host must be a hostname or an IP address; port, database and credentials are separate fields",
			name)
	}
	return nil
}

// hostname accepts what a host may be and nothing else. Listing the characters
// to refuse left tabs, quotes, semicolons and control bytes through, so
// `db.internal;password=secret` was a valid host. A positive rule cannot have
// that gap: anything not named here is refused, including the next separator
// somebody's driver decides to honour.
var hostname = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func validateCredentialSource(name string, source CredentialSource) error {
	switch {
	case source.Kind != CredentialVaultDatabaseRole:
		return fmt.Errorf("connector: sql %s needs a credential source of kind %q",
			name, CredentialVaultDatabaseRole)
	case strings.TrimSpace(source.VaultInstance) == "":
		return fmt.Errorf("connector: sql %s credential source needs a vault instance", name)
	case strings.TrimSpace(source.Mount) == "":
		return fmt.Errorf("connector: sql %s credential source needs a mount", name)
	case strings.TrimSpace(source.Role) == "":
		return fmt.Errorf("connector: sql %s credential source needs a role", name)
	}
	return nil
}

/*
ValidateBindings checks what one instance alone cannot answer.

A binding names another instance, so it can only be judged against the set.
Every failure here is closed: a reference that is missing, disabled, tokenless,
ambiguous or pointing at another connector refuses the configuration rather
than deferring to a runtime that would discover it on the first approved query.

This does not replace revalidation in the runtime. Configuration changes after
it is written, and a vault disabled tomorrow must not authorise a query today
because the binding was valid when it was saved.
*/
func ValidateBindings(instances []Instance) error {
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		switch instance.Connector {
		case "sql":
			if err := resolveVaultBinding(instance, instances); err != nil {
				return err
			}
		case "gravitee":
			if err := resolveGraviteeVaultBinding(instance, instances); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveVaultBinding(sql Instance, instances []Instance) error {
	_, err := boundVault(sql, sql.SQL.CredentialSource.VaultInstance, instances)
	return err
}

func resolveGraviteeVaultBinding(gravitee Instance, instances []Instance) error {
	vault, err := boundVault(gravitee, gravitee.Gravitee.CredentialSource.VaultInstance, instances)
	if err != nil {
		return err
	}
	if _, ok := allowedPath(vault.Vault.AllowedPathPrefixes, gravitee.Gravitee.CredentialSource.Path); !ok {
		return fmt.Errorf("connector: gravitee %s credential path is outside vault instance %q",
			gravitee.Name, vault.Name)
	}
	return nil
}

func boundVault(consumer Instance, wanted string, instances []Instance) (Instance, error) {
	var found []Instance
	for _, candidate := range instances {
		if candidate.Name != wanted || !candidate.Scope.Contains(consumer.Scope) {
			continue
		}
		found = append(found, candidate)
	}
	switch {
	case len(found) == 0:
		return Instance{}, fmt.Errorf(
			"connector: %s %s names vault instance %q, which is not configured for its scope",
			consumer.Connector, consumer.Name, wanted)
	case len(found) > 1:
		return Instance{}, fmt.Errorf(
			"connector: %s %s names vault instance %q, which is ambiguous across scopes",
			consumer.Connector, consumer.Name, wanted)
	}
	vault := found[0]
	switch {
	case vault.Connector != "vault":
		return Instance{}, fmt.Errorf("connector: %s %s names %q, which is a %s instance and not a vault",
			consumer.Connector, consumer.Name, wanted, vault.Connector)
	case !vault.Enabled:
		return Instance{}, fmt.Errorf("connector: %s %s names vault instance %q, which is disabled",
			consumer.Connector, consumer.Name, wanted)
	// A dynamic credential is short-lived, which bounds how long a stolen one
	// is useful and does nothing about it being read in flight. The token that
	// mints it travels the same way, so plain HTTP is the same disclosure one
	// step earlier.
	case !strings.HasPrefix(vault.Vault.Address, "https://"):
		return Instance{}, fmt.Errorf(
			"connector: %s %s names vault instance %q, which must use https to supply credentials",
			consumer.Connector, consumer.Name, wanted)
	case !vault.TokenPresent():
		return Instance{}, fmt.Errorf("connector: %s %s names vault instance %q, which has no token",
			consumer.Connector, consumer.Name, wanted)
	}
	return vault, nil
}
