package auth

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// shortHash derives a stable, readable identifier from an input.
//
// Base32 without padding rather than hex: the same entropy in fewer
// characters, and no ambiguity between characters when an operator reads an
// identifier out of a log to someone else.
func shortHash(in string) string {
	sum := sha256.Sum256([]byte(in))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return strings.ToLower(enc[:16])
}
