package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func hostedServer(url string) domain.MCPServer {
	return domain.MCPServer{Name: "github-developer", Transport: domain.TransportHTTP, URL: url}
}

func appCreds() domain.MCPCredentials {
	return domain.MCPCredentials{GitHubApp: &domain.MCPGitHubApp{AppID: 7, InstallationID: 9, PrivateKey: "k"}}
}

// The hosted GitHub MCP server, reached as a GitHub App on github.com, is the
// one registration whose create_pull_request answer proves a link.
func TestHostedGitHubProvenance_theHostedServerAsAnApp_isEstablished(t *testing.T) {
	t.Parallel()

	for _, url := range []string{"https://api.githubcopilot.com/mcp/", "https://api.githubcopilot.com/mcp", "https://api.githubcopilot.com/mcp/x/repos"} {
		if got := domain.HostedGitHubProvenance(hostedServer(url), appCreds()); got != domain.ProvenanceHostedGitHubApp {
			t.Errorf("%s as an App = %q, want %q", url, got, domain.ProvenanceHostedGitHubApp)
		}
	}
}

// Fail closed: anything the registration cannot establish mechanically is no
// provenance, and a link through it waits for a person.
func TestHostedGitHubProvenance_anythingElse_establishesNothing(t *testing.T) {
	t.Parallel()

	pat := domain.MCPCredentials{Headers: map[string]string{"Authorization": "Bearer ghp_x"}}
	ghe := appCreds()
	ghe.GitHubApp.APIBaseURL = "https://ghe.example.com/api/v3"
	noInstall := appCreds()
	noInstall.GitHubApp.InstallationID = 0
	cases := map[string]struct {
		server domain.MCPServer
		creds  domain.MCPCredentials
	}{
		"a personal token":       {hostedServer("https://api.githubcopilot.com/mcp/"), pat},
		"no credential":          {hostedServer("https://api.githubcopilot.com/mcp/"), domain.MCPCredentials{}},
		"an Enterprise App":      {hostedServer("https://api.githubcopilot.com/mcp/"), ghe},
		"an App with no install": {hostedServer("https://api.githubcopilot.com/mcp/"), noInstall},
		"plain http":             {hostedServer("http://api.githubcopilot.com/mcp/"), appCreds()},
		"another host":           {hostedServer("https://github-mcp.example.com/mcp/"), appCreds()},
		"a look-alike host":      {hostedServer("https://api.githubcopilot.com.evil.example/mcp/"), appCreds()},
		"an explicit port":       {hostedServer("https://api.githubcopilot.com:8443/mcp/"), appCreds()},
		"userinfo in the URL":    {hostedServer("https://x@api.githubcopilot.com/mcp/"), appCreds()},
		"another path":           {hostedServer("https://api.githubcopilot.com/mcpx/"), appCreds()},
		"a local process":        {domain.MCPServer{Name: "github", Transport: domain.TransportStdio, Command: "github-mcp-server"}, appCreds()},
	}
	for name, c := range cases {
		if got := domain.HostedGitHubProvenance(c.server, c.creds); got != "" {
			t.Errorf("%s: provenance = %q, want none", name, got)
		}
	}
}
