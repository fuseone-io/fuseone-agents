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
	// From and Events are the published source and the deliveries that start
	// a run; see domain.WebhookTrigger.
	From   string
	Events []string
}

// Webhooks is where paths and their secrets are kept.
type Webhooks interface {
	// Find returns the hook at a path, or ErrNoHook.
	Find(ctx context.Context, path string) (Hook, error)
	// Verify reports whether the secret matches the one stored for the path.
	// A GitHub hook's secret is never accepted this way: it signs, it is not
	// sent.
	Verify(ctx context.Context, path, secret string) (bool, error)
	// SigningSecret opens a GitHub hook's secret, to check a signature with.
	SigningSecret(ctx context.Context, path string) ([]byte, error)
	// Rotate replaces the secret and returns the new one, once — only while
	// the path still belongs to the agent that asked, or ErrNoHook.
	Rotate(ctx context.Context, r Rotation) (string, error)
	// Sync reconciles an agent's declared webhooks.
	Sync(ctx context.Context, agent domain.AgentID, scope domain.Scope, hooks []domain.WebhookTrigger) error
	// ForAgent lists what an agent declares, for the console.
	ForAgent(ctx context.Context, agent domain.AgentID) ([]Hook, error)
}

// Rotation is a request for a new secret: the path, the agent whose path the
// caller was authorised for, and who asked when.
//
// The agent is part of the request because the check that the caller may
// rotate this path and the write happen at two moments. A path that changed
// hands between them must not arm the new owner's hook and hand its key to
// somebody authorised for the old one.
type Rotation struct {
	Path  string
	Agent domain.AgentID
	By    domain.UserID
	At    time.Time
}

// ErrNoHook means nothing is declared at that path.
var ErrNoHook = errors.New("trigger: no webhook at that path")

// ErrNotArmed means the path exists but nobody has generated its secret.
var ErrNotArmed = errors.New("trigger: the webhook has no secret yet")

// ErrNoVault means a GitHub hook's secret cannot be kept: it has to be read
// back to check signatures, and without the master key it could only be
// stored in the clear.
var ErrNoVault = errors.New("trigger: a GitHub webhook's secret needs the master key to be stored")

// ErrNotGitHub means the path is an ordinary hook, which keeps no signing
// secret.
var ErrNotGitHub = errors.New("trigger: the webhook does not come from GitHub")

// ErrPathTaken means another agent already answers on that path.
//
// Refused rather than transferred: the sender configured against it holds a
// secret, and moving the path would point that secret at a different agent
// without anybody having to be told.
var ErrPathTaken = errors.New("trigger: another agent already owns that path")

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
