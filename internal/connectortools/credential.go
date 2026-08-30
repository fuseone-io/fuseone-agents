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
IssuedCredential is what the Vault client answers: a credential and the lease
that has to be given back, both already unprintable.

The client builds this from a wire struct it never lets out, so there is no
moment between the network and here where an exported field holds a password.
*/
type IssuedCredential struct {
	credential Credential
	leaseID    string
	ttlSeconds int
}

func (i IssuedCredential) String() string               { return redacted }
func (i IssuedCredential) Format(f fmt.State, _ rune)   { _, _ = f.Write([]byte(redacted)) }
func (i IssuedCredential) GoString() string             { return redacted }
func (i IssuedCredential) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

/*
Issuance is safe provenance, and the lease id is not part of it.

What a run may record: which instances answered, which binding, and how long
the lease lasts. The lease id is the handle that revokes it and it describes
the internal shape of the lease, so it lives on Authority instead — where the
only thing that can be done with it is give the lease back.
*/
type Issuance struct {
	SQLInstance     string `json:"sqlInstance"`
	VaultInstance   string `json:"vaultInstance"`
	Mount           string `json:"mount"`
	Role            string `json:"role"`
	LeaseTTLSeconds int    `json:"leaseTtlSeconds"`
}

/*
Authority is what an approved operation receives: something to connect with,
something to say about it, and a way to give it back.

The lease id, the vault configuration and the token stay private together,
because the only thing a caller needs them for is Revoke. Handing them out as
data would make a lease id available to whatever records the operation, and
slice 4 is exactly where that record gets written.
*/
type Authority struct {
	credential Credential
	issuance   Issuance
	lease      lease
}

type lease struct {
	id     string
	config VaultConfig
	token  string
	vault  CredentialIssuer
}

func (a Authority) Credential() Credential { return a.credential }
func (a Authority) Issuance() Issuance     { return a.issuance }

// Revoke gives the lease back. The short TTL remains the backstop: a failure
// here is reported and is not a reason to keep the connection.
func (a Authority) Revoke(ctx context.Context) error {
	if a.lease.id == "" || a.lease.vault == nil {
		return nil
	}
	return a.lease.vault.RevokeLease(ctx, a.lease.config, a.lease.token, a.lease.id)
}

func (a Authority) String() string               { return redacted }
func (a Authority) Format(f fmt.State, _ rune)   { _, _ = f.Write([]byte(redacted)) }
func (a Authority) GoString() string             { return redacted }
func (a Authority) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// Configuration and VaultTokens are what the resolver needs, declared here
// because this is what uses them. The token reader is separate and asked for
// one instance: revealing every stored token to answer one question would
// widen the blast radius of this call to the whole installation.
type Configuration interface {
	ConfiguredInstances(ctx context.Context) ([]ConfiguredInstance, error)
}

// VaultTokens reveals one token, named and scoped. Settings.Instances reveals
// every enabled instance's token to answer any question, so this asks for the
// one already resolved — and the method name is deliberately one that
// Settings.Instances cannot satisfy by accident.
type VaultTokens interface {
	RevealVaultToken(ctx context.Context, instance ConfiguredInstance) (string, error)
}

type CredentialIssuer interface {
	IssueDatabaseCredential(
		ctx context.Context, cfg VaultConfig, token, mount, role string,
	) (IssuedCredential, error)
	RevokeLease(ctx context.Context, cfg VaultConfig, token, leaseID string) error
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
) (Authority, error) {
	instances, err := r.config.ConfiguredInstances(ctx)
	if err != nil {
		return Authority{}, err
	}
	sql, err := oneInstance(instances, "sql", sqlInstance, scope)
	if err != nil {
		return Authority{}, err
	}
	source := sql.SQL.CredentialSource
	if source.Kind != CredentialVaultDatabaseRole {
		return Authority{}, fmt.Errorf(
			"%w: %s has no vault database role binding", ErrNoCredentialSource, sqlInstance)
	}
	vault, err := oneInstance(instances, "vault", source.VaultInstance, scope)
	if err != nil {
		return Authority{}, err
	}
	token, err := r.tokens.RevealVaultToken(ctx, vault)
	if err != nil {
		// A stopped run and a configuration that cannot answer are different
		// facts, and flattening them would tell an operator to fix settings
		// while the truth is that somebody cancelled.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Authority{}, err
		}
		return Authority{}, fmt.Errorf("%w: vault %s has no usable token",
			ErrNoCredentialSource, vault.Name)
	}
	issued, err := r.vault.IssueDatabaseCredential(
		ctx, vault.Vault, token, source.Mount, source.Role)
	if err != nil {
		return Authority{}, issuanceError(err, sqlInstance)
	}
	return Authority{
		credential: issued.credential,
		issuance: Issuance{
			SQLInstance: sql.Name, VaultInstance: vault.Name,
			Mount: source.Mount, Role: source.Role,
			LeaseTTLSeconds: issued.ttlSeconds,
		},
		lease: lease{
			id: issued.leaseID, config: vault.Vault, token: token, vault: r.vault,
		},
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
	// Errors this package created carry nothing from Vault, so they pass
	// through: a cancelled run and an answer missing a lease id are facts the
	// caller has to tell apart, and only what Vault said is flattened.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrCredentialIncomplete) {
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
	instances []ConfiguredInstance, connector, name string, scope domain.Scope,
) (ConfiguredInstance, error) {
	var found []ConfiguredInstance
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
		return ConfiguredInstance{}, fmt.Errorf("%w: no enabled %s instance %q covers this run",
			ErrNoCredentialSource, connector, name)
	default:
		return ConfiguredInstance{}, fmt.Errorf("%w: %s instance %q is ambiguous for this run",
			ErrNoCredentialSource, connector, name)
	}
}
