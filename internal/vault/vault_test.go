package vault_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/vault"
)

func newVault(t *testing.T) *vault.Vault {
	t.Helper()
	key, err := vault.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	v, err := vault.FromBase64(key, "k1")
	if err != nil {
		t.Fatalf("FromBase64: %v", err)
	}
	return v
}

func TestSealOpen_roundTrips(t *testing.T) {
	t.Parallel()

	v := newVault(t)
	const secret = "sk-ant-not-a-real-key"

	ct, nonce, err := v.Seal([]byte(secret), "provider/anthropic")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if string(ct) == secret {
		t.Fatal("the ciphertext is the plaintext")
	}

	got, err := v.Open(ct, nonce, "provider/anthropic")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != secret {
		t.Errorf("Open = %q, want the original secret", got)
	}
}

func TestSeal_sameSecretTwice_producesDifferentCiphertext(t *testing.T) {
	t.Parallel()

	v := newVault(t)
	a, _, _ := v.Seal([]byte("same"), "ctx")
	b, _, _ := v.Seal([]byte("same"), "ctx")

	// A fresh nonce each time. Identical ciphertexts would let anyone with
	// read access to the table tell which providers share a credential.
	if string(a) == string(b) {
		t.Error("sealing the same secret twice produced identical ciphertext")
	}
}

func TestOpen_underADifferentContext_fails(t *testing.T) {
	t.Parallel()

	v := newVault(t)
	ct, nonce, _ := v.Seal([]byte("crm token"), "mcp/crm")

	// The context binds a ciphertext to the record it belongs to. Moving a row
	// from a low-privilege credential onto a high-privilege one must not
	// promote it.
	if _, err := v.Open(ct, nonce, "mcp/payments"); !errors.Is(err, vault.ErrCiphertext) {
		t.Errorf("Open = %v, want %v", err, vault.ErrCiphertext)
	}
}

func TestOpen_tamperedCiphertext_isDetected(t *testing.T) {
	t.Parallel()

	v := newVault(t)
	ct, nonce, _ := v.Seal([]byte("crm token"), "mcp/crm")
	ct[0] ^= 0xff

	// GCM authenticates as well as encrypts: an attacker with write access
	// cannot flip bits and have the platform use the altered value.
	if _, err := v.Open(ct, nonce, "mcp/crm"); !errors.Is(err, vault.ErrCiphertext) {
		t.Errorf("Open = %v, want %v", err, vault.ErrCiphertext)
	}
}

func TestOpen_withAnotherInstallationsKey_fails(t *testing.T) {
	t.Parallel()

	ct, nonce, _ := newVault(t).Seal([]byte("secret"), "ctx")

	if _, err := newVault(t).Open(ct, nonce, "ctx"); !errors.Is(err, vault.ErrCiphertext) {
		t.Errorf("Open = %v, want %v", err, vault.ErrCiphertext)
	}
}

func TestNew_rejectsAKeyOfTheWrongLength(t *testing.T) {
	t.Parallel()

	// Silently padding or truncating a short key is how an installation ends
	// up believing it has 256 bits of protection that it does not.
	if _, err := vault.New([]byte("too short"), "k1"); !errors.Is(err, vault.ErrBadKey) {
		t.Errorf("New = %v, want %v", err, vault.ErrBadKey)
	}
	if _, err := vault.New(nil, "k1"); !errors.Is(err, vault.ErrNoKey) {
		t.Errorf("New = %v, want %v", err, vault.ErrNoKey)
	}
}

func TestSeal_emptySecret_storesNothing(t *testing.T) {
	t.Parallel()

	ct, nonce, err := newVault(t).Seal(nil, "ctx")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	// "No credential set" must be distinguishable from "a credential that
	// happens to be empty", or the console cannot tell an operator which
	// providers still need configuring.
	if ct != nil || nonce != nil {
		t.Error("an empty secret produced a stored ciphertext")
	}
}

func TestMask_showsPresenceWithoutTheValue(t *testing.T) {
	t.Parallel()

	got := vault.Mask("sk-ant-api03-SECRETVALUE9xyz")

	// The console shows that a credential is set and roughly which one; it
	// never shows the value back, not even to whoever entered it. A secret
	// readable from the console is a secret that leaks through a screen share.
	if got == "sk-ant-api03-SECRETVALUE9xyz" {
		t.Fatal("Mask returned the secret")
	}
	if len(got) < 8 || got[len(got)-4:] != "9xyz" {
		t.Errorf("Mask = %q, want a masked value keeping the last four", got)
	}
	if vault.Mask("") != "" {
		t.Error("an unset credential should mask to nothing at all")
	}
}
