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
App. A tool named create_pull_request proves nothing on its own: any
registered server may offer one, and answer with the address of a pull request
it never opened. So the catalogue records, per registration, the provenance it
can establish mechanically, and the engine stamps it with the pull request it
strictly parsed; the fold and both stores refuse a stamp without it.

The answer's content stays untrusted — the Curator's default for
create_pull_request, which the factory keeps — and taints the run as any
untrusted result does (review round 2). The stamp is a separate, sanitised
projection: one canonical reference and one provenance value, trusted as proof
because of where it came from and how it was read, never because the content
was. The two trusts are not to be merged.

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
// tool_returned step: the pull request the strict parser read from the
// answer, and the provenance of the server that answered. Recorded only under
// that provenance — whatever the content's own label, which stays on the step.
type PullRequestStamp struct {
	Ref WorkItemRef    `json:"ref"`
	Via ToolProvenance `json:"via"`
}

// TrustedProof reports whether the stamp may prove a link: it exists and the
// hosted GitHub provenance stands behind it. A capability of the stamp, not of
// the answer — the answer's content label never makes or unmakes it.
func (s *PullRequestStamp) TrustedProof() bool {
	return s != nil && s.Via == ProvenanceHostedGitHubApp
}
