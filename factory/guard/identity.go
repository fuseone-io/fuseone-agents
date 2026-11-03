package guard

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrTokenRejected marks a bearer GitHub does not accept.
var ErrTokenRejected = errors.New("github rejected the token")

// viewerQuery asks GitHub whose token this is. An installation token's viewer
// is its App's bot account, and the bot's databaseId is the same user id
// FACTORY_QA_BOT_ID already pins: immutable, unlike a login.
const viewerQuery = `query { viewer { databaseId } }`

func (c *Client) viewerID(ctx context.Context) (int64, error) {
	var out struct {
		Data struct {
			Viewer struct {
				DatabaseID int64 `json:"databaseId"`
			} `json:"viewer"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}
	code, err := c.graphQL(ctx, graphQLBody{Query: viewerQuery}, &out)
	switch {
	case err != nil:
		return 0, err
	case code == http.StatusUnauthorized:
		return 0, ErrTokenRejected
	case code != http.StatusOK || len(out.Errors) > 0 || out.Data.Viewer.DatabaseID <= 0:
		return 0, fmt.Errorf("%w: POST graphql: viewer: status %d, %d errors", ErrGitHub, code, len(out.Errors))
	}
	return out.Data.Viewer.DatabaseID, nil
}

// identityTTL is how long a token's identity is trusted without asking
// again: well under the hour an installation token lives.
const identityTTL = 10 * time.Minute

// maxIdentities bounds the cache; past it, expired entries are dropped, and
// if none were, all are.
const maxIdentities = 256

type cachedIdentity struct {
	bot   int64
	until time.Time
}

// identities learns the bot behind a token and remembers it briefly. The
// cache is keyed by the token's SHA-256, never the token, and nothing here
// logs either.
type identities struct {
	gh    *Client
	clock Clock

	mu    sync.Mutex
	cache map[[sha256.Size]byte]cachedIdentity
}

func (i *identities) bot(ctx context.Context, token string) (int64, error) {
	key := sha256.Sum256([]byte(token))
	now := i.clock.Now()
	i.mu.Lock()
	hit, ok := i.cache[key]
	i.mu.Unlock()
	if ok && now.Before(hit.until) {
		return hit.bot, nil
	}
	id, err := i.gh.viewerID(withCallToken(ctx, token))
	if err != nil {
		return 0, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.cache) >= maxIdentities {
		i.sweep(now)
	}
	i.cache[key] = cachedIdentity{bot: id, until: now.Add(identityTTL)}
	return id, nil
}

func (i *identities) sweep(now time.Time) {
	for k, v := range i.cache {
		if !now.Before(v.until) {
			delete(i.cache, k)
		}
	}
	if len(i.cache) >= maxIdentities {
		clear(i.cache)
	}
}

// passThrough serves each request the surface of the identity behind its
// bearer. The surface is chosen from the token alone: never from the path, a
// header or the token's textual form. Each role has its own MCP handler, so
// a session opened by one identity is unknown to every other role's handler.
type passThrough struct {
	ids      *identities
	bots     map[int64]Role
	surfaces map[Role]http.Handler
}

// NewPassThroughHandler builds the FACTORY_MCP_AUTH=github-app-token handler.
// gh must take its token from the call (RequestTokenSource).
func NewPassThroughHandler(cfg Config, gh *Client, clock Clock) (http.Handler, error) {
	p := &passThrough{
		ids:      &identities{gh: gh, clock: clock, cache: map[[sha256.Size]byte]cachedIdentity{}},
		bots:     cfg.RoleBots,
		surfaces: map[Role]http.Handler{},
	}
	for _, role := range cfg.Roles() {
		server, err := NewServer(cfg, role, gh)
		if err != nil {
			return nil, err
		}
		p.surfaces[role] = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	}
	return p, nil
}

func (p *passThrough) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := bearer(r.Header)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	bot, err := p.ids.bot(r.Context(), token)
	switch {
	case errors.Is(err, ErrTokenRejected):
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	case err != nil:
		http.Error(w, "identity unavailable", http.StatusBadGateway)
		return
	}
	surface, ok := p.surfaces[p.bots[bot]]
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	surface.ServeHTTP(w, r)
}
