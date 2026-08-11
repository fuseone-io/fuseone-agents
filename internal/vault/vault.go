// Package vault encrypts the credentials the administration area stores.
//
// Provider keys and MCP tokens are the platform's most valuable secrets: one
// of them is enough to spend an installation's budget or reach a customer's
// CRM. They are encrypted before they reach the database, under a key that
// lives outside it, so a stolen dump is not a stolen credential.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	ErrNoKey      = errors.New("vault: no master key configured")
	ErrBadKey     = errors.New("vault: master key must be 32 bytes")
	ErrCiphertext = errors.New("vault: ciphertext is not valid")
)

// KeyEnv names the variable holding the master key, base64-encoded.
const KeyEnv = "FUSEONE_MASTER_KEY"

// Vault seals and opens secrets with AES-256-GCM.
//
// GCM rather than plain CBC or CTR because it authenticates as well as
// encrypts: an attacker with write access to the database cannot flip bits in
// a stored key and have the platform use the altered value.
type Vault struct {
	aead cipher.AEAD
	// keyID lets an installation rotate. A record carries the identifier of
	// the key that sealed it, so old records stay readable while new ones use
	// the new key — rotation without a stop-the-world re-encryption.
	keyID string
}

// New builds a vault from a 32-byte key.
func New(key []byte, keyID string) (*Vault, error) {
	if len(key) == 0 {
		return nil, ErrNoKey
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w, got %d", ErrBadKey, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault: %w", err)
	}
	if keyID == "" {
		keyID = "default"
	}
	return &Vault{aead: aead, keyID: keyID}, nil
}

// FromBase64 builds a vault from the encoded form the environment carries.
func FromBase64(encoded, keyID string) (*Vault, error) {
	if encoded == "" {
		return nil, ErrNoKey
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("vault: master key is not valid base64: %w", err)
	}
	return New(key, keyID)
}

// GenerateKey mints a master key for a fresh installation.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("vault: generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Seal encrypts a secret and returns the ciphertext and its nonce.
//
// context binds the ciphertext to where it belongs — the setting's scope, kind
// and name. A row copied from one provider's record to another's fails to
// open, so an attacker with database write access cannot promote a
// low-privilege credential by moving it.
func (v *Vault) Seal(secret []byte, context string) (ciphertext, nonce []byte, err error) {
	if len(secret) == 0 {
		return nil, nil, nil
	}

	nonce = make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("vault: nonce: %w", err)
	}

	ciphertext = v.aead.Seal(nil, nonce, secret, []byte(v.keyID+"|"+context))
	return ciphertext, nonce, nil
}

// Open decrypts a secret sealed under the same context.
func (v *Vault) Open(ciphertext, nonce []byte, context string) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	if len(nonce) != v.aead.NonceSize() {
		return nil, fmt.Errorf("%w: nonce is %d bytes, want %d",
			ErrCiphertext, len(nonce), v.aead.NonceSize())
	}

	plain, err := v.aead.Open(nil, nonce, ciphertext, []byte(v.keyID+"|"+context))
	if err != nil {
		// Never echo the underlying reason: "authentication failed" and
		// "wrong key" are both an attacker's oracle.
		return nil, ErrCiphertext
	}
	return plain, nil
}

// KeyID identifies the key in use, for recording alongside sealed records.
func (v *Vault) KeyID() string { return v.keyID }

// Mask renders a secret for display.
//
// The administration area shows whether a credential is present and roughly
// which one it is; it never shows the value back, not even to the operator who
// set it. A secret that can be read out of the console is a secret that leaks
// through a screen share.
func Mask(secret string) string {
	switch n := len(secret); {
	case n == 0:
		return ""
	case n <= 8:
		return "••••••••"
	default:
		return "••••••••" + secret[n-4:]
	}
}
