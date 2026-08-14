package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

/*
The administrator's own password.

Break-glass, and worth saying plainly: identity comes from the customer's
identity provider and always will. This is the one account that works when the
provider does not — or does not exist yet, which is the state every
installation starts in and, until now, the state an operator could be locked
out by. The setup token is single use, so an administrator whose session ended
before they registered a provider had no door left at all.

PBKDF2-HMAC-SHA256 from the standard library, not argon2 from a module. Argon2
is the better answer against a GPU, and DE-01 is "one binary, one PostgreSQL,
nothing else required" — a dependency for one function on a break-glass path
is a supply chain an air-gapped installation now has to accept. The iteration
count is OWASP's current figure for this construction, and the parameters
travel with the hash so raising it later does not invalidate anything already
set.
*/

// MinPasswordLength is the floor, in characters.
//
// Length rather than a character-class rule: "P@ssw0rd!" satisfies every such
// rule and is on every list, and the rules mostly teach people to put a 1 at
// the end. Twelve is what NIST asks for when a second factor is not in play.
const MinPasswordLength = 12

// MaxPasswordLength stops a megabyte of text becoming a way to make the
// server do six hundred thousand rounds of SHA-256 over it.
const MaxPasswordLength = 256

// iterations is the work factor. Named rather than inline because it is
// written into every hash and read back out of the old ones.
const iterations = 600_000

const (
	saltBytes = 16
	keyBytes  = 32
	scheme    = "pbkdf2-sha256"
)

// ErrPasswordTooShort and ErrPasswordTooLong are refusals a person can act on.
var (
	ErrPasswordTooShort = fmt.Errorf(
		"a password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong = fmt.Errorf(
		"a password may be at most %d characters", MaxPasswordLength)
)

// HashPassword seals a password for storage.
//
// The result carries its own algorithm, cost and salt, so a stored hash says
// how to check itself and nothing outside has to remember.
func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLength {
		return "", ErrPasswordTooLong
	}

	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyBytes)
	if err != nil {
		return "", fmt.Errorf("auth: derive: %w", err)
	}

	return strings.Join([]string{
		scheme, strconv.Itoa(iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

// PasswordMatches reports whether a password produces a stored hash.
//
// Anything it cannot read is a refusal. A stored value that does not parse is
// a broken row or a tampered one, and neither is a reason to let somebody in.
func PasswordMatches(hash, password string) bool {
	stored, salt, rounds, err := parseHash(hash)
	if err != nil || len(password) > MaxPasswordLength {
		return false
	}

	key, err := pbkdf2.Key(sha256.New, password, salt, rounds, len(stored))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(stored, key) == 1
}

func parseHash(hash string) (key, salt []byte, rounds int, err error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != scheme {
		return nil, nil, 0, errors.New("auth: not a password hash")
	}
	if rounds, err = strconv.Atoi(parts[1]); err != nil || rounds < 1 {
		return nil, nil, 0, errors.New("auth: password hash has no cost")
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[2]); err != nil {
		return nil, nil, 0, fmt.Errorf("auth: password salt: %w", err)
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[3]); err != nil {
		return nil, nil, 0, fmt.Errorf("auth: password key: %w", err)
	}
	if len(key) == 0 {
		return nil, nil, 0, errors.New("auth: password hash is empty")
	}
	return key, salt, rounds, nil
}
