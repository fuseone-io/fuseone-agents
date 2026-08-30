package connectortools

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrNoCredentialSource means the configuration cannot answer this run:
	// nothing stored, disabled, out of scope, or two rows answering one name.
	// Every one of them fails closed, before Vault is contacted.
	ErrNoCredentialSource = errors.New("connector: no credential source for this run")
	// ErrCredentialIncomplete means Vault answered without what a connection
	// and a revocation both need. There is no fallback: a partial answer is a
	// refusal, not a reason to reach for a static credential.
	ErrCredentialIncomplete = errors.New("connector: vault returned an incomplete credential")
)

/*
Credential is a short-lived database credential, and a type that cannot be
printed, logged or serialised by accident.

The fields are unexported and every rendering Go offers is overridden —
String, Format and MarshalJSON — because the ways a secret reaches a log are
not the ways anybody chooses. `%v` on a struct that happens to contain one,
a JSON response that grew a field, an error built with `%+v`: none of those is
a decision, and all of them are how this leaks.

Only the SQL runtime reads the bytes, through the accessors below.
*/
type Credential struct {
	username string
	password string
}

func (c Credential) Username() string { return c.username }
func (c Credential) Password() string { return c.password }

const redacted = "[redacted credential]"

func (c Credential) String() string { return redacted }

// Format covers the verbs String does not: %#v and %+v reach the struct
// itself, and a struct of strings prints its fields.
func (c Credential) Format(f fmt.State, verb rune) { _, _ = f.Write([]byte(redacted)) }

func (c Credential) GoString() string { return redacted }

func (c Credential) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

/*
Issuance is what may be said about a credential without saying the credential.

Enough to record provenance and to revoke: which binding answered, how long
the lease lasts, and the lease's own id. The username and password are not
here, and adding a field to this struct is a decision to publish it.
*/
type Issuance struct {
	SQLInstance     string `json:"sqlInstance"`
	VaultInstance   string `json:"vaultInstance"`
	Mount           string `json:"mount"`
	Role            string `json:"role"`
	LeaseID         string `json:"leaseId"`
	LeaseTTLSeconds int    `json:"leaseTtlSeconds"`
}

// Configuration and VaultTokens are what the resolver needs, declared here
// because this is what uses them. The token reader is separate and asked for
// one instance: revealing every stored token to answer one question would
// widen the blast radius of this call to the whole installation.
type Configuration interface {
	Instances(ctx context.Context) ([]Instance, error)
}

type VaultTokens interface {
	VaultToken(ctx context.Context, name string, scope domain.Scope) (string, error)
}

type CredentialIssuer interface {
	IssueDatabaseCredential(
		ctx context.Context, cfg VaultConfig, token, mount, role string,
	) (VaultDatabaseCredential, error)
}

type CredentialResolver struct {
	config Configuration
	tokens VaultTokens
	vault  CredentialIssuer
}

func NewCredentialResolver(
	config Configuration, tokens VaultTokens, vault CredentialIssuer,
) *CredentialResolver {
	return &CredentialResolver{config: config, tokens: tokens, vault: vault}
}

/*
Resolve turns a SQL instance name and a run scope into database authority.

Those two arguments are the whole input. Mount and role come from the stored
binding rather than from the caller, because a caller that could pass them
could compose an arbitrary Vault path with a SQL destination — the composition
this design exists to prevent, and one no approval would catch.

The configuration is read again here rather than trusted from the write that
validated it. A vault disabled since then, an instance moved to another scope,
a second row that made a name ambiguous: all of them refuse now, and all of
them refuse before Vault is contacted.
*/
func (r *CredentialResolver) Resolve(
	ctx context.Context, sqlInstance string, scope domain.Scope,
) (Credential, Issuance, error) {
	instances, err := r.config.Instances(ctx)
	if err != nil {
		return Credential{}, Issuance{}, err
	}
	sql, err := oneInstance(instances, "sql", sqlInstance, scope)
	if err != nil {
		return Credential{}, Issuance{}, err
	}
	source := sql.SQL.CredentialSource
	if source.Kind != CredentialVaultDatabaseRole {
		return Credential{}, Issuance{}, fmt.Errorf(
			"%w: %s has no vault database role binding", ErrNoCredentialSource, sqlInstance)
	}
	vault, err := oneInstance(instances, "vault", source.VaultInstance, scope)
	if err != nil {
		return Credential{}, Issuance{}, err
	}
	token, err := r.tokens.VaultToken(ctx, vault.Name, vault.Scope)
	if err != nil {
		return Credential{}, Issuance{}, fmt.Errorf("%w: vault %s has no usable token",
			ErrNoCredentialSource, vault.Name)
	}
	issued, err := r.vault.IssueDatabaseCredential(
		ctx, vault.Vault, token, source.Mount, source.Role)
	if err != nil {
		return Credential{}, Issuance{}, issuanceError(err, sqlInstance)
	}
	if err := issued.valid(); err != nil {
		return Credential{}, Issuance{}, err
	}
	return Credential{username: issued.Username, password: issued.Password},
		Issuance{
			SQLInstance: sql.Name, VaultInstance: vault.Name,
			Mount: source.Mount, Role: source.Role,
			LeaseID: issued.LeaseID, LeaseTTLSeconds: issued.LeaseTTLSeconds,
		}, nil
}

/*
issuanceError says that issuance failed and nothing about what Vault said.

A Vault error body can quote the path, the role, the policy and sometimes the
value; wrapping it would put all of that into a log line and an API response.
Cancellation is the exception and passes through as itself, because a caller
has to tell a stopped run from a broken configuration.
*/
func issuanceError(err error, sqlInstance string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("connector: vault refused to issue a credential for %s", sqlInstance)
}

/*
oneInstance is exactly one enabled instance of a connector, by name, whose
scope contains the run's.

Two rows answering one name refuse rather than resolve. The configuration
already refuses to store that, and this refuses to act on it anyway: a
resolver that picked the narrower or the first would reintroduce the
choice-by-order that the stored rules removed, at the one moment where the
choice hands out authority.
*/
func oneInstance(
	instances []Instance, connector, name string, scope domain.Scope,
) (Instance, error) {
	var found []Instance
	for _, instance := range instances {
		if instance.Connector != connector || instance.Name != name || !instance.Enabled {
			continue
		}
		if !instance.Scope.Contains(scope) {
			continue
		}
		found = append(found, instance)
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return Instance{}, fmt.Errorf("%w: no enabled %s instance %q covers this run",
			ErrNoCredentialSource, connector, name)
	default:
		return Instance{}, fmt.Errorf("%w: %s instance %q is ambiguous for this run",
			ErrNoCredentialSource, connector, name)
	}
}
