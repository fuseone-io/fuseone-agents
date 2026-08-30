package connectortools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func company(name string) domain.Scope { return domain.Scope{Company: domain.CompanyID(name)} }

func area(c, a string) domain.Scope {
	return domain.Scope{Company: domain.CompanyID(c), Area: domain.AreaID(a)}
}

func vaultInstance(name string, scope domain.Scope) Instance {
	return Instance{
		Connector: "vault", Name: name, Scope: scope, Enabled: true, Token: "t",
		Vault: VaultConfig{
			Address: "https://vault.internal", Mount: "database",
			AllowedPathPrefixes: []string{"database/creds"},
		},
	}
}

func sqlInstance(scope domain.Scope, source CredentialSource) Instance {
	return Instance{
		Connector: "sql", Name: "app-x", Scope: scope, Enabled: true,
		SQL: SQLConfig{
			Driver: SQLDriverPostgres,
			Host:   "db.internal", Port: 5432, Database: "appx",
			CredentialSource: source,
		},
	}
}

func vaultRole(instance string) CredentialSource {
	return CredentialSource{
		Kind: CredentialVaultDatabaseRole, VaultInstance: instance,
		Mount: "database", Role: "app-x-readonly",
	}
}

/*
A SQL instance takes its authority from a binding, never from a token of its
own.

The connector exists so that no database password is stored in FuseOne. An
instance carrying a token would be that password under another name, and the
refusal has to happen at configuration time: by the time a run needs authority
it is too late to discover that somebody pasted one in.
*/
func TestValidateInstance_sqlRefusesATokenOfItsOwn(t *testing.T) {
	t.Parallel()

	instance := sqlInstance(area("acme", "platform"), vaultRole("prod"))
	instance.Token = "postgres-password"

	err := ValidateInstance(instance)
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("ValidateInstance err = %v, want a refusal naming the token", err)
	}
}

// Vault holds a token because it authenticates to Vault. SQL does not, so the
// requirement is asked per connector rather than assumed for every instance.
func TestRequiresToken_isAskedPerConnector(t *testing.T) {
	t.Parallel()

	if !RequiresToken("vault") {
		t.Error("vault authenticates with a token and must require one")
	}
	if RequiresToken("sql") {
		t.Error("sql takes authority from its binding and must not require a token")
	}
}

/*
Structured host, port and database, never a connection string.

A DSN is one field that can carry a password, and a validator that tries to
recognise one is a validator that has to keep up with every driver's spelling.
Refusing the shape is the only version that stays true.
*/
func TestValidateInstanceConfig_sqlRefusesCredentialsInsideItsAddressing(t *testing.T) {
	t.Parallel()

	for _, host := range []string{
		"user:secret@db.internal",
		"postgres://user:secret@db.internal:5432/appx",
		"db.internal/appx?password=secret",
	} {
		instance := sqlInstance(area("acme", "platform"), vaultRole("prod"))
		instance.SQL.Host = host
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("host %q was accepted; it can carry a credential", host)
		}
	}
}

func TestValidateBindings_vaultInAWiderScopeCoversTheSQLInstance(t *testing.T) {
	t.Parallel()

	err := ValidateBindings([]Instance{
		vaultInstance("prod", company("acme")),
		sqlInstance(area("acme", "platform"), vaultRole("prod")),
	})
	if err != nil {
		t.Fatalf("ValidateBindings err = %v, want a wider vault scope to cover", err)
	}
}

func TestValidateBindings_vaultInTheSameScopeCoversTheSQLInstance(t *testing.T) {
	t.Parallel()

	scope := area("acme", "platform")
	if err := ValidateBindings([]Instance{
		vaultInstance("prod", scope), sqlInstance(scope, vaultRole("prod")),
	}); err != nil {
		t.Fatalf("ValidateBindings err = %v, want the same scope to cover", err)
	}
}

/*
A vault the SQL instance's scope does not sit inside cannot lend it authority.

This is the cross-talk property #15 is about, applied to credentials: a run in
one area must not reach a credential source configured for another, and the
refusal belongs at configuration time as well as at runtime.
*/
func TestValidateBindings_refusesAVaultThatDoesNotContainTheSQLScope(t *testing.T) {
	t.Parallel()

	err := ValidateBindings([]Instance{
		vaultInstance("prod", area("acme", "payments")),
		sqlInstance(area("acme", "platform"), vaultRole("prod")),
	})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("ValidateBindings err = %v, want a refusal naming the scope", err)
	}
}

/*
One name in two scopes is not a reference, it is a question.

Resolving it by picking the narrower or the first would make the binding mean
whatever the configuration order happened to be, and an operator reading the
YAML could not tell which vault answers.
*/
func TestValidateBindings_refusesANameThatTwoVaultsAnswerTo(t *testing.T) {
	t.Parallel()

	err := ValidateBindings([]Instance{
		vaultInstance("prod", company("acme")),
		vaultInstance("prod", area("acme", "platform")),
		sqlInstance(area("acme", "platform"), vaultRole("prod")),
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ValidateBindings err = %v, want an ambiguous reference refused", err)
	}
}

func TestValidateBindings_refusesAMissingDisabledOrTokenlessVault(t *testing.T) {
	t.Parallel()

	disabled := vaultInstance("prod", company("acme"))
	disabled.Enabled = false
	tokenless := vaultInstance("prod", company("acme"))
	tokenless.Token = ""

	for name, instances := range map[string][]Instance{
		"missing":   {sqlInstance(area("acme", "platform"), vaultRole("prod"))},
		"disabled":  {disabled, sqlInstance(area("acme", "platform"), vaultRole("prod"))},
		"tokenless": {tokenless, sqlInstance(area("acme", "platform"), vaultRole("prod"))},
	} {
		if err := ValidateBindings(instances); err == nil {
			t.Errorf("%s vault was accepted as a credential source", name)
		}
	}
}

// The binding names a vault. Pointing it at an instance of another connector
// is a configuration mistake that would otherwise surface as a runtime failure
// on the first approved query.
func TestValidateBindings_refusesAReferenceToAnotherConnector(t *testing.T) {
	t.Parallel()

	other := Instance{Connector: "smtp", Name: "prod", Scope: company("acme"), Enabled: true}
	err := ValidateBindings([]Instance{
		other, sqlInstance(area("acme", "platform"), vaultRole("prod")),
	})
	if err == nil || !strings.Contains(err.Error(), "vault") {
		t.Fatalf("ValidateBindings err = %v, want a refusal naming the connector", err)
	}
}

/*
Configuring SQL does not offer the model a tool it cannot run.

The catalogue shape is MaturityPlanned: there is no SQL runtime yet. Tool
entries are built from a connector's operations without asking, so without
this an operator configuring an instance today would put three uncallable
tools on an agent's surface.
*/
func TestToolEntries_aPlannedConnectorOffersNoTool(t *testing.T) {
	t.Parallel()

	entries := toolEntriesFor([]Instance{
		sqlInstance(area("acme", "platform"), vaultRole("prod")),
		vaultInstance("prod", area("acme", "platform")),
	})
	for _, entry := range entries {
		if strings.HasPrefix(string(entry.ID), "sql.") {
			t.Errorf("planned connector offered %s", entry.ID)
		}
	}
	if len(entries) == 0 {
		t.Fatal("no entries at all; this test would pass on anything")
	}
}

/*
Nothing about a SQL instance that leaves the platform can carry a credential.

The response type is built field by field rather than by copying the struct,
and this is the accuser for that: a field added to SQLConfig later has to be
a decision to expose it. The marker below is what a leak would look like.
*/
func TestSQLConfig_carriesNoFieldThatCouldHoldACredential(t *testing.T) {
	t.Parallel()

	const marker = "SQL-CONFIG-CANARY"
	instance := sqlInstance(area("acme", "platform"), vaultRole("prod"))
	instance.Token = marker

	if err := ValidateInstance(instance); err == nil {
		t.Fatal("a sql instance carrying a token was accepted")
	}
	// The refusal names the field, never the value: an error is copied into
	// logs and API responses, and a message quoting the token would put it in
	// both.
	if err := ValidateInstance(instance); strings.Contains(err.Error(), marker) {
		t.Errorf("the refusal repeated the token: %v", err)
	}
}

// Saving a configuration must not reach the system it configures. An operator
// writes down an intention before the thing it points at is necessarily up,
// and a validator that dials would make that impossible.
func TestValidateBindings_readsOnlyTheConfiguration(t *testing.T) {
	t.Parallel()

	vault := vaultInstance("prod", company("acme"))
	vault.Vault.Address = "https://vault.invalid.example"

	if err := ValidateBindings([]Instance{
		vault, sqlInstance(area("acme", "platform"), vaultRole("prod")),
	}); err != nil {
		t.Fatalf("ValidateBindings err = %v; an unreachable address must not fail configuration", err)
	}
}

/*
What is written down is what comes back.

Every other test here builds an Instance in memory, so none of them notices a
field the stored shape drops. This one goes through the encoding: a binding
that survives a PUT and returns empty on the next read is a configuration an
operator believes they made.
*/
func TestSettingValue_roundTripsTheSQLBinding(t *testing.T) {
	t.Parallel()

	original := sqlInstance(area("acme", "platform"), vaultRole("prod"))
	value, err := SettingValue(original)
	if err != nil {
		t.Fatalf("SettingValue: %v", err)
	}
	back, err := SettingInstance(settings.Setting{
		Name: original.Name, Value: value, Enabled: true,
		ScopeKind: settings.ScopeArea, Scope: original.Scope,
	})
	if err != nil {
		t.Fatalf("SettingInstance: %v", err)
	}
	if !reflect.DeepEqual(back.SQL, original.SQL) {
		t.Fatalf("SQL = %+v, want %+v", back.SQL, original.SQL)
	}
}

// The stored shape holds no secret, so a round trip must not invent one.
func TestSettingValue_storesNoToken(t *testing.T) {
	t.Parallel()

	instance := vaultInstance("prod", company("acme"))
	instance.Token = "VAULT-TOKEN-CANARY"
	value, err := SettingValue(instance)
	if err != nil {
		t.Fatalf("SettingValue: %v", err)
	}
	if strings.Contains(string(value), "VAULT-TOKEN-CANARY") {
		t.Fatalf("the stored value carries the token: %s", value)
	}
}

/*
A vault whose token is held by the secret store still counts as having one.

Configured() deliberately does not reveal credentials, so the instances it
returns carry an empty Token. Asking resolveVaultBinding to read that field
would refuse every real configuration as tokenless while accepting the
in-memory fixtures these tests build — which is exactly how this passed
review the first time.
*/
func TestValidateBindings_acceptsAVaultWhoseTokenIsHeldElsewhere(t *testing.T) {
	t.Parallel()

	vault := vaultInstance("prod", company("acme"))
	vault.Token = ""
	vault.HasToken = true

	if err := ValidateBindings([]Instance{
		vault, sqlInstance(area("acme", "platform"), vaultRole("prod")),
	}); err != nil {
		t.Fatalf("ValidateBindings err = %v, want a stored token to count", err)
	}
}

func TestPlainHost_refusesEverythingThatIsNotAHostOrAnAddress(t *testing.T) {
	t.Parallel()

	for _, host := range []string{
		"db.internal;password=secret",
		"db.internal\tuser=admin",
		"db.internal\npassword=secret",
		`db."internal"`,
		"db internal",
		"user:secret@db.internal",
		"postgres://db.internal/appx",
		"",
	} {
		instance := sqlInstance(area("acme", "platform"), vaultRole("prod"))
		instance.SQL.Host = host
		if err := ValidateInstanceConfig(instance); err == nil {
			t.Errorf("host %q was accepted", host)
		}
	}
	for _, host := range []string{"db.internal", "db-1.internal", "10.0.0.4", "::1"} {
		instance := sqlInstance(area("acme", "platform"), vaultRole("prod"))
		instance.SQL.Host = host
		if err := ValidateInstanceConfig(instance); err != nil {
			t.Errorf("host %q was refused: %v", host, err)
		}
	}
}
