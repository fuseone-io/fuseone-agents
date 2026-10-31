package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// githubAppFromRequest keeps absent and empty apart: nil leaves the stored App
// alone, and an empty object is somebody removing it.
func githubAppFromRequest(in *openapi.MCPGitHubApp) *domain.MCPGitHubApp {
	if in == nil {
		return nil
	}
	app := domain.MCPGitHubApp{
		AppID:          valueOr(in.AppId),
		InstallationID: valueOr(in.InstallationId),
		PrivateKey:     valueOr(in.PrivateKey),
		APIBaseURL:     valueOr(in.ApiBaseURL),
	}
	if in.Repositories != nil {
		app.Repositories = append([]string(nil), (*in.Repositories)...)
	}
	return &app
}

// githubAppToResponse renders the App's non-secret half. The summary it reads
// has no key to render; HasPrivateKey is true because an App is only stored
// with one.
func githubAppToResponse(in *domain.MCPGitHubAppSummary) *openapi.MCPGitHubAppSummary {
	if in == nil {
		return nil
	}
	out := &openapi.MCPGitHubAppSummary{
		AppId: in.AppID, InstallationId: in.InstallationID, HasPrivateKey: true,
		ApiBaseURL: someString(in.APIBaseURL),
	}
	if len(in.Repositories) > 0 {
		repos := append([]string(nil), in.Repositories...)
		out.Repositories = &repos
	}
	return out
}
