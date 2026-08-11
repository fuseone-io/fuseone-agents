package trigger

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// A webhook is a door into the installation with an agent behind it.
//
// It is authenticated by a secret the operator generates and the caller sends,
// compared in constant time, and it is closed until somebody generates that
// secret — a declared path that accepted anything until it was configured
// would be worse than no webhook at all.
//
// Replay is handled by the same thing that handles a redelivery: the caller
// names the delivery, and the same name is always the same run. A captured
// request replayed by somebody who should not have it opens nothing.

// Hook is a configured webhook path.
type Hook struct {
	Path    string
	Agent   domain.AgentID
	Scope   domain.Scope
	Armed   bool
	Rotated time.Time
	By      domain.UserID
}

// Webhooks is where paths and their secrets are kept.
type Webhooks interface {
	// Find returns the hook at a path, or ErrNoHook.
	Find(ctx context.Context, path string) (Hook, error)
	// Verify reports whether the secret matches the one stored for the path.
	Verify(ctx context.Context, path, secret string) (bool, error)
	// Rotate replaces the secret and returns the new one, once.
	Rotate(ctx context.Context, path string, by domain.UserID, at time.Time) (string, error)
	// Sync reconciles an agent's declared paths.
	Sync(ctx context.Context, agent domain.AgentID, scope domain.Scope, paths []string) error
	// ForAgent lists what an agent declares, for the console.
	ForAgent(ctx context.Context, agent domain.AgentID) ([]Hook, error)
}

// ErrNoHook means nothing is declared at that path.
var ErrNoHook = errors.New("trigger: no webhook at that path")

// ErrNotArmed means the path exists but nobody has generated its secret.
var ErrNotArmed = errors.New("trigger: the webhook has no secret yet")

// NewSecret returns a secret and its hash. The secret is returned once and
// never stored: what is kept cannot be turned back into what the caller sends.
func NewSecret() (secret string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("trigger: generate secret: %w", err)
	}
	secret = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(secret))
	return secret, sum[:], nil
}

// MatchesSecret compares in constant time. A comparison that returns early on
// the first wrong byte tells an attacker how much of the secret they have.
func MatchesSecret(stored []byte, offered string) bool {
	if len(stored) == 0 {
		return false
	}
	sum := sha256.Sum256([]byte(offered))
	return subtle.ConstantTimeCompare(stored, sum[:]) == 1
}

// DeliveryKey names one delivery of one hook.
//
// The path is part of it so two agents cannot collide on a delivery id their
// senders chose independently.
func DeliveryKey(path, delivery string) string {
	return fmt.Sprintf("hook:%s:%s", path, delivery)
}
