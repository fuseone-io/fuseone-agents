// Package githubapp authenticates as a GitHub App: it signs the App's JWT
// with its private key and exchanges it for a short-lived installation access
// token, refreshed before it expires.
//
// The standard library is enough. An App JWT is three base64url segments and
// one RS256 signature; crypto/rsa and crypto/x509 do the cryptography, and a
// JWT library would add a dependency to do string concatenation.
package githubapp

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Every key error is a fixed sentence. The input is a private key, and an
// error that quoted what it could not parse would put the key in a log line.
var (
	ErrKeyNotPEM     = errors.New("github app: the private key is not a PEM block")
	ErrKeyNotRSA     = errors.New("github app: the private key is not an RSA key")
	ErrKeyUnreadable = errors.New("github app: the private key could not be parsed")
)

const (
	// jwtBackdate absorbs clock skew between this worker and GitHub, as
	// GitHub's own documentation recommends.
	jwtBackdate = 60 * time.Second
	// jwtLifetime is counted from the backdated iat. GitHub refuses a JWT
	// that lives longer than ten minutes.
	jwtLifetime = 10 * time.Minute
)

// ParsePrivateKey reads the App's private key as PKCS#1 ("RSA PRIVATE KEY",
// what GitHub downloads) or PKCS#8 ("PRIVATE KEY").
func ParsePrivateKey(text string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(text))
	if block == nil {
		return nil, ErrKeyNotPEM
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Not wrapped: the parser's message is about the bytes it read.
		return nil, ErrKeyUnreadable
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrKeyNotRSA
	}
	return key, nil
}

// appJWT signs the JWT that authenticates as the App itself.
func appJWT(key *rsa.PrivateKey, appID int64, now time.Time) (string, error) {
	issued := now.Add(-jwtBackdate)
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("github app: encode JWT header: %w", err)
	}
	claims, err := json.Marshal(struct {
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
		Issuer    string `json:"iss"`
	}{
		IssuedAt:  issued.Unix(),
		ExpiresAt: issued.Add(jwtLifetime).Unix(),
		Issuer:    strconv.FormatInt(appID, 10),
	})
	if err != nil {
		return "", fmt.Errorf("github app: encode JWT claims: %w", err)
	}
	signing := segment(header) + "." + segment(claims)
	digest := sha256.Sum256([]byte(signing))
	// PKCS#1 v1.5 is deterministic; the random source is ignored.
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("github app: sign JWT: %w", err)
	}
	return signing + "." + segment(sig), nil
}

func segment(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
