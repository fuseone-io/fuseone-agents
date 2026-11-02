package domain

import (
	"net/url"
	"strings"
)

/*
Where a create_pull_request answer came from, and whether it may prove a link
(#204, slice 2b, review round 1).

A pull request named by an answer proves a link without a person only if the
answer is the hosted GitHub MCP server's, reached as the installation's GitHub
App, and the result was not labelled untrusted. A tool named create_pull_request
proves nothing on its own: any registered server may offer one, and a server
classified untrusted may answer with the address of a pull request it never
opened. So the catalogue records, per registration, the provenance it can
establish mechanically, and the engine stamps it with the pull request; the
fold and both stores refuse a stamp without it, or on an untrusted step.

The signal is the narrowest the registration holds: an https endpoint on
api.githubcopilot.com under /mcp — the hosted server the factory registers per
role — and a GitHub App credential on github.com, so the answer is GitHub's,
about the App's own installation. A local github-mcp-server, a proxy, a personal token or any
other endpoint establishes nothing, and its links wait for a person.
*/

// ToolProvenance names what the catalogue could establish about the server a
// call was dispatched to. Empty is nothing.
type ToolProvenance string

// ProvenanceHostedGitHubApp is the hosted GitHub MCP server reached as a
// GitHub App.
const ProvenanceHostedGitHubApp ToolProvenance = "github_hosted_mcp_app"

const hostedGitHubMCPHost = "api.githubcopilot.com"

// HostedGitHubProvenance is the provenance a registration establishes: the
// hosted GitHub MCP server as a GitHub App, or nothing.
func HostedGitHubProvenance(server MCPServer, creds MCPCredentials) ToolProvenance {
	app := creds.GitHubApp
	if server.TransportOf() != TransportHTTP || app == nil || app.AppID == 0 || app.InstallationID == 0 ||
		app.APIBaseURL != "" {
		return ""
	}
	u, err := url.Parse(server.URL)
	if err != nil || u.Scheme != "https" || u.Host != hostedGitHubMCPHost || u.User != nil {
		return ""
	}
	if u.Path != "/mcp" && !strings.HasPrefix(u.Path, "/mcp/") {
		return ""
	}
	return ProvenanceHostedGitHubApp
}

// PullRequestStamp is what the engine records on a create_pull_request's
// tool_returned step: the pull request the answer named, and the provenance of
// the server that answered. Recorded only for a trusted result.
type PullRequestStamp struct {
	Ref WorkItemRef    `json:"ref"`
	Via ToolProvenance `json:"via"`
}
