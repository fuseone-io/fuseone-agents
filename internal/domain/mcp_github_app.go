package domain

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// DefaultGitHubAPIBase is github.com's REST API. GitHub Enterprise Server
// installations name their own, which ends in /api/v3.
const DefaultGitHubAPIBase = "https://api.github.com"

/*
MCPGitHubApp is a GitHub App a remote MCP server is reached as.

Not a token. A token is the thing to send; this is what a worker mints one
from: the App's private key signs a short JWT, GitHub exchanges it for an
installation access token, and that token expires within the hour. Nothing
but these fields is ever stored — the minted token lives in the memory of the
worker that minted it.

The private key is the whole credential. Every way this type is rendered for a
person — fmt verbs and slog alike — leaves it out, so a log line that prints a
credential by mistake still prints no key.
*/
type MCPGitHubApp struct {
	AppID          int64  `json:"appID,omitempty"`
	InstallationID int64  `json:"installationID,omitempty"`
	PrivateKey     string `json:"privateKey,omitempty"`
	// Repositories narrows the installation token to these repositories, by
	// name. Empty means everything the installation was granted.
	Repositories []string `json:"repositories,omitempty"`
	// APIBaseURL is the REST API root. Empty means github.com.
	APIBaseURL string `json:"apiBaseURL,omitempty"`
}

func (g MCPGitHubApp) Empty() bool {
	return g.AppID == 0 && g.InstallationID == 0 && g.PrivateKey == "" &&
		len(g.Repositories) == 0 && g.APIBaseURL == ""
}

func (g MCPGitHubApp) Equal(other MCPGitHubApp) bool {
	return g.AppID == other.AppID &&
		g.InstallationID == other.InstallationID &&
		g.PrivateKey == other.PrivateKey &&
		slices.Equal(g.Repositories, other.Repositories) &&
		g.APIBaseURL == other.APIBaseURL
}

// APIBase is the REST API root without a trailing slash.
func (g MCPGitHubApp) APIBase() string {
	base := strings.TrimRight(strings.TrimSpace(g.APIBaseURL), "/")
	if base == "" {
		return DefaultGitHubAPIBase
	}
	return base
}

// Format renders the App for every fmt verb without its key.
func (g MCPGitHubApp) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "github_app{app=%d installation=%d repositories=%d key=%s}",
		g.AppID, g.InstallationID, len(g.Repositories), keyPresence(g.PrivateKey))
}

// LogValue renders the App for slog without its key.
func (g MCPGitHubApp) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int64("app", g.AppID),
		slog.Int64("installation", g.InstallationID),
		slog.Int("repositories", len(g.Repositories)),
		slog.String("key", keyPresence(g.PrivateKey)),
	)
}

func keyPresence(key string) string {
	if key == "" {
		return "absent"
	}
	return "configured"
}

func cloneGitHubApp(g *MCPGitHubApp) *MCPGitHubApp {
	if g == nil || g.Empty() {
		return nil
	}
	out := *g
	out.Repositories = append([]string(nil), g.Repositories...)
	return &out
}

// mergeGitHubApp folds a write onto the stored App. A write without a key
// keeps the stored one: correcting a repository list must not demand pasting
// a private key again.
func mergeGitHubApp(stored, given *MCPGitHubApp) *MCPGitHubApp {
	out := cloneGitHubApp(given)
	if out != nil && out.PrivateKey == "" && stored != nil {
		out.PrivateKey = stored.PrivateKey
	}
	return out
}
