package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

/*
SessionIdentity is who a session opened with these credentials speaks as: a
digest that moves when somebody deliberately changes them, and only then.

A server that fixes who is calling at initialize keeps that for the session's
life, so a session opened under one identity must not carry a call made under
another. An OAuth grant is its generation — moved by a person's write, kept
across the worker's rotation (KeptGeneration) — and the client it is for; a
token or headers are what they say. Digested so it can sit beside a session
without holding the credential.
*/
func (c MCPCredentials) SessionIdentity() string {
	parts := []string{"token", c.Token}
	if c.OAuth != nil && !c.OAuth.Empty() {
		parts = append(parts, "oauth", c.OAuth.Generation, c.OAuth.ClientID, c.OAuth.TokenURL,
			c.OAuth.SignInConfig)
	}
	names := make([]string, 0, len(c.Headers))
	for name := range c.Headers {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		parts = append(parts, "header", name, c.Headers[name])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
