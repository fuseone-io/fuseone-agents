package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

const (
	// RefreshBefore is how long before expiry a token is replaced. GitHub's
	// installation tokens live an hour; five minutes is room for a slow mint
	// and a long tool call that started just before the deadline.
	RefreshBefore = 5 * time.Minute
	// stillValid is the least time left for a token to be kept when a refresh
	// fails. Below it the request could reach GitHub after expiry.
	stillValid = 30 * time.Second
)

// Clock is where a Source reads the time, so tests can drive expiry.
type Clock interface {
	Now() time.Time
}

/*
Source hands out an installation token for one App installation.

The token lives in this worker's memory and nowhere else: it expires within the
hour, every worker can mint its own from the stored key, and persisting it
would add a second secret to the vault for no reader that needs it.

One mint at a time. The lock is held across the request, so twenty calls
arriving at the refresh moment wait for one token rather than minting twenty.
*/
type Source struct {
	mu      sync.Mutex
	minter  minter
	clock   Clock
	current token
}

// NewSource parses the App's key up front: a key that cannot be read is
// reported once, at connect time, rather than at every call.
func NewSource(app domain.MCPGitHubApp, client *http.Client, clock Clock) (*Source, error) {
	key, err := ParsePrivateKey(app.PrivateKey)
	if err != nil {
		return nil, err
	}
	if app.AppID <= 0 || app.InstallationID <= 0 {
		return nil, fmt.Errorf("github app: the App id and installation id are both required")
	}
	return &Source{minter: minter{client: client, app: app, key: key}, clock: clock}, nil
}

// Token returns a token with more than RefreshBefore left, minting one when
// the token in hand is closer to expiry than that.
func (s *Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	if s.current.value != "" && now.Before(s.current.expiresAt.Add(-RefreshBefore)) {
		return s.current.value, nil
	}
	next, err := s.minter.mint(ctx, now)
	if err != nil {
		// A refresh that fails early leaves a token that still works. Keep
		// it; the next call tries the mint again.
		if s.current.value != "" && now.Before(s.current.expiresAt.Add(-stillValid)) {
			return s.current.value, nil
		}
		return "", err
	}
	s.current = next
	return next.value, nil
}
