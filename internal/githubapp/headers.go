package githubapp

import (
	"errors"
	"maps"
	"net/http"
	"slices"
)

/*
configHeaders are the only headers that may ride beside an App: the optional
headers the hosted GitHub MCP server documents, each the equivalent of a local
server flag (github/github-mcp-server, docs/remote-server.md, "Optional
Headers"). Canonicalised the way net/http compares them.

An allowlist, not a ban on Authorization: any other name — X-API-Key, Cookie, a
vendor's own — can carry a second secret beside the App's bearer, and a
server that acts as the App must send its identity and nothing else.
*/
var configHeaders = []string{
	"X-Mcp-Insiders",
	"X-Mcp-Lockdown",
	"X-Mcp-Readonly",
	"X-Mcp-Tools",
	"X-Mcp-Toolsets",
}

// ErrHeaderNotAllowed is a header beside an App outside the configuration
// headers. It names no header and no value; the caller knows what it sent.
var ErrHeaderNotAllowed = errors.New(
	"github app: beside a GitHub App only the hosted GitHub MCP server's configuration headers are accepted " +
		"— X-MCP-Toolsets, X-MCP-Tools, X-MCP-Readonly, X-MCP-Lockdown and X-MCP-Insiders; " +
		"the App supplies the credential")

// CheckConfigHeaders refuses a header map that names anything but a
// configuration header, in any letter case.
func CheckConfigHeaders(headers map[string]string) error {
	for name := range headers {
		if !isConfigHeader(name) {
			return ErrHeaderNotAllowed
		}
	}
	return nil
}

// ConfigHeaders is headers with only the configuration headers kept: the
// runtime's own filter, so a row stored before the check, or by a path that
// skipped it, still sends nothing beside the App's identity.
func ConfigHeaders(headers map[string]string) map[string]string {
	out := maps.Clone(headers)
	maps.DeleteFunc(out, func(name, _ string) bool { return !isConfigHeader(name) })
	return out
}

func isConfigHeader(name string) bool {
	return slices.Contains(configHeaders, http.CanonicalHeaderKey(name))
}
