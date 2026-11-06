package vault_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/vault"
)

func testKeyed(t *testing.T, fill byte) *vault.Vault {
	t.Helper()
	v, err := vault.New(bytes.Repeat([]byte{fill}, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return v
}

func TestFingerprint_sameMaterialAndContext_isStable(t *testing.T) {
	t.Parallel()
	v := testKeyed(t, 1)
	if a, b := v.Fingerprint([]byte("s3cret"), "here"), v.Fingerprint([]byte("s3cret"), "here"); a != b || a == "" {
		t.Errorf("fingerprints %q and %q", a, b)
	}
}

// Somebody holding the database and a guess must not be able to check it.
func TestFingerprint_withoutTheMasterKey_cannotBeComputed(t *testing.T) {
	t.Parallel()
	material := []byte("s3cret")
	got := testKeyed(t, 1).Fingerprint(material, "here")
	if got == testKeyed(t, 2).Fingerprint(material, "here") {
		t.Error("two master keys gave one fingerprint")
	}
	sum := sha256.Sum256(material)
	if strings.Contains(got, hex.EncodeToString(sum[:])[:16]) {
		t.Errorf("%q is a plain hash of the secret", got)
	}
}

func TestFingerprint_sameMaterialElsewhere_differs(t *testing.T) {
	t.Parallel()
	v := testKeyed(t, 1)
	if v.Fingerprint([]byte("s3cret"), "here") == v.Fingerprint([]byte("s3cret"), "there") {
		t.Error("one secret on two resources gave one fingerprint, which lets a guess sealed on one test the other")
	}
}

// Where the context ends and the material starts has to be unambiguous.
func TestFingerprint_contextAndMaterialSplitDifferently_differs(t *testing.T) {
	t.Parallel()
	v := testKeyed(t, 1)
	if v.Fingerprint([]byte("bc"), "a") == v.Fingerprint([]byte("c"), "ab") {
		t.Error("moving a byte across the boundary gave the same fingerprint")
	}
}

func TestFingerprint_anyMaterial_isOpaqueAndFixedWidth(t *testing.T) {
	t.Parallel()
	got := testKeyed(t, 1).Fingerprint([]byte("s3cret"), "here")
	if !strings.HasPrefix(got, "fp_") || len(got) != len("fp_")+32 {
		t.Errorf("fingerprint = %q, want fp_ and 128 bits of hex", got)
	}
}
