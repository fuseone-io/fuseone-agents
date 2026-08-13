package vault_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/vault"
)

/*
No key and a wrong key are different facts.

An installation that has not configured a provider yet has no key, and must
still start: configuring one is what the administration area is for. An
installation whose key is the wrong length has somebody's mistake in it, and
starting quietly means the console looks healthy until the day a credential is
saved.

The first lab install made the case: the key was hex where base64 was meant,
the API served without a word, and the workers crash-looped beside it. The
error naming the problem was in the log nobody was reading.
*/
func TestFromEnv_noKeySet_saysSoDistinctly(t *testing.T) {
	t.Setenv(vault.KeyEnv, "")

	_, err := vault.FromEnv("primary")
	if !errors.Is(err, vault.ErrNoKey) {
		t.Fatalf("err = %v, want ErrNoKey — an unconfigured installation must be able to start", err)
	}
}

func TestFromEnv_keySetButWrongLength_isNotMistakenForNoKey(t *testing.T) {
	// 32 bytes of hex, which is 64 characters, which base64-decodes to 48.
	t.Setenv(vault.KeyEnv, "6d9a1f5c8e2b47a03f16d8c5b9e4a72f1c0b3d6e8a5f2740c1e9b8d7a6f5c4b3")

	_, err := vault.FromEnv("primary")
	if err == nil {
		t.Fatal("a 48-byte key was accepted")
	}
	if errors.Is(err, vault.ErrNoKey) {
		t.Fatal("a key that is set but wrong was reported as no key at all")
	}
}
