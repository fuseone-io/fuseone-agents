package admin

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/githubapp"
	"github.com/fuseone/agents/internal/netguard"
)

// ErrBadGitHubApp is a GitHub App credential the worker could never mint
// with. Refused at save time: stored, it would surface later as an
// unreachable server, far from the form where the mistake was made.
var ErrBadGitHubApp = errors.New("admin: unusable GitHub App credential")

/*
storedGitHubApp is the half of a GitHub App a listing may show.

The sealed document holds all of it; this copy exists so the console can say
which App and installation a server acts as without opening the vault, which
an ordinary read never does. Written in the same fold as the sealed document,
so the two cannot disagree.
*/
type storedGitHubApp struct {
	AppID          int64    `json:"appID"`
	InstallationID int64    `json:"installationID"`
	Repositories   []string `json:"repositories,omitempty"`
	APIBaseURL     string   `json:"apiBaseURL,omitempty"`
}

// validGitHubApp checks what minting needs, with reasons that never quote the
// key: the key errors are fixed sentences, and nothing here formats input.
// serverURL is where the installation token goes, so it is held to TLS too.
func validGitHubApp(app *domain.MCPGitHubApp, serverURL string) error {
	switch {
	case app == nil:
		return nil
	case app.AppID <= 0:
		return fmt.Errorf("%w: the App id is required", ErrBadGitHubApp)
	case app.InstallationID <= 0:
		return fmt.Errorf("%w: the installation id is required", ErrBadGitHubApp)
	}
	if _, err := githubapp.ParsePrivateKey(app.PrivateKey); err != nil {
		return fmt.Errorf("%w: %w", ErrBadGitHubApp, err)
	}
	if err := githubapp.RequireTLS(serverURL); err != nil {
		return fmt.Errorf("%w: server URL: %w", ErrBadGitHubApp, err)
	}
	if app.APIBaseURL == "" {
		return nil
	}
	if err := netguard.ValidateHTTPURL(app.APIBaseURL); err != nil {
		return fmt.Errorf("%w: API base URL: %w", ErrBadGitHubApp, err)
	}
	if err := githubapp.RequireTLS(app.APIBaseURL); err != nil {
		return fmt.Errorf("%w: API base URL: %w", ErrBadGitHubApp, err)
	}
	return nil
}

func storedGitHubAppOf(app *domain.MCPGitHubApp) *storedGitHubApp {
	if app == nil || app.Empty() {
		return nil
	}
	return &storedGitHubApp{
		AppID: app.AppID, InstallationID: app.InstallationID,
		Repositories: app.Repositories, APIBaseURL: app.APIBaseURL,
	}
}

func (s *storedGitHubApp) summary() *domain.MCPGitHubAppSummary {
	if s == nil {
		return nil
	}
	return &domain.MCPGitHubAppSummary{
		AppID: s.AppID, InstallationID: s.InstallationID,
		Repositories: s.Repositories, APIBaseURL: s.APIBaseURL,
	}
}
