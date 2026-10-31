package httpapi

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

const pastedKey = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEAsecret\n-----END RSA PRIVATE KEY-----\n"

// The App arrives at the store whole, the way it was typed.
func TestPutMCPServer_passesTheGitHubAppToTheStore(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	repos := []string{"fuseone-agents"}
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{
		Name: "github",
		Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptr(openapi.TransportHttp),
			Url:       ptr("https://api.githubcopilot.com/mcp/"),
			GithubApp: &openapi.MCPGitHubApp{
				AppId: ptr(int64(5550600)), InstallationId: ptr(int64(91)),
				PrivateKey: ptr(pastedKey), Repositories: &repos,
				ApiBaseURL: ptr("https://ghe.example.com/api/v3"),
			},
		},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, ok := resp.(openapi.PutMCPServer204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	got := admin.putGitHubApp
	if got == nil || got.AppID != 5550600 || got.InstallationID != 91 || got.PrivateKey != pastedKey ||
		!slices.Equal(got.Repositories, repos) || got.APIBaseURL != "https://ghe.example.com/api/v3" {
		t.Fatalf("github app = %v, want the request's App passed to admin", got)
	}
}

// An empty object is a removal, and must reach the store as one rather than
// be dropped as "not mentioned".
func TestPutMCPServer_anEmptyGitHubApp_reachesTheStoreAsARemoval(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	if _, err := serverWith(t, admin).PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{
		Name: "github",
		Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptr(openapi.TransportHttp), Url: ptr("https://api.githubcopilot.com/mcp/"),
			GithubApp: &openapi.MCPGitHubApp{},
		},
	}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if admin.putGitHubApp == nil || !admin.putGitHubApp.Empty() {
		t.Fatalf("github app = %v, want an empty App: the removal", admin.putGitHubApp)
	}
}

// The listing names the App and installation and says the key is configured;
// the rendered response has nowhere for the key to be.
func TestListIntegrations_aGitHubApp_listsItsConfigurationAndNeverTheKey(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{servers: []domain.MCPServer{{
		Name: "github", Transport: domain.TransportHTTP, URL: "https://api.githubcopilot.com/mcp/",
		HasSecret: true, Enabled: true,
		GitHubApp: &domain.MCPGitHubAppSummary{
			AppID: 5550600, InstallationID: 91, Repositories: []string{"fuseone-agents"},
		},
	}}}
	resp, err := serverWith(t, admin).ListIntegrations(as(domain.RoleCurator), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	body, ok := resp.(openapi.ListIntegrations200JSONResponse)
	if !ok || len(body.McpServers) != 1 {
		t.Fatalf("response = %#v, want one server", resp)
	}
	app := body.McpServers[0].GithubApp
	if app == nil || app.AppId != 5550600 || app.InstallationId != 91 || !app.HasPrivateKey ||
		app.Repositories == nil || (*app.Repositories)[0] != "fuseone-agents" {
		t.Fatalf("githubApp = %+v, want its configuration and a configured key", app)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "privateKey") {
		t.Fatalf("the listing has a field for the key: %s", raw)
	}
}
