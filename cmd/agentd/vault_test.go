package main

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/vault"
)

/*
No master key and a wrong master key are different situations.

The console serves without one: this process reports that a credential exists
and never opens one, and refusing to boot would stop an installation that has
not configured a provider yet from starting at all — when configuring one is
what the administration area is for. A key that is set and wrong is the
opposite: serving happily beside workers crash-looping on the same value is a
configuration mistake dressed as a broken image.

Which means the caller has to be able to tell them apart, and it tells them
apart by the sentinel.
*/
func TestOpenVault_noKeyConfigured_keepsTheSentinel(t *testing.T) {
	t.Setenv(vault.KeyEnv, "")

	_, err := openVault()
	if !errors.Is(err, vault.ErrNoKey) {
		t.Fatalf("err = %v, want it to answer errors.Is(ErrNoKey)", err)
	}
}

func TestOpenVault_aKeyThatIsNot32Bytes_isNotReadAsUnset(t *testing.T) {
	// 32 bytes of hex reads like a key and decodes to 48 — the mistake people
	// actually make, and the one that must not boot as though nothing was set.
	t.Setenv(vault.KeyEnv, "6d6f646f")

	_, err := openVault()
	if errors.Is(err, vault.ErrNoKey) {
		t.Fatalf("err = %v, want a wrong key to be a different answer from no key", err)
	}
	if err == nil {
		t.Fatal("a key that is not 32 bytes was accepted")
	}
}
