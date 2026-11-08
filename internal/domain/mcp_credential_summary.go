package domain

import (
	"encoding/json"
	"net/url"
	"slices"
	"sort"
)

/*
What a read may say about a sealed credential document.

The document stays sealed whole; this is worked out from it when one server is
read, and stored nowhere. Field by field:

Shown, because a client has to compare them and none of them authenticates
anybody: header names, variable names, the token address, the client id, the
token type and the scopes.

Fingerprinted, because they are or may be secrets: the token, header and variable
values, the configuration file, the client secret and the App's private key. A
header's value is in here even for one like X-MCP-Toolsets: nothing can tell a
configuration header from a credential by reading its value, and the rule for a
field that is sometimes a secret is to treat it as one.

Neither, because a worker changes them with nobody having decided anything: the
access token, the refresh token and when the access token expires. Counted, every
server on OAuth would read as drifted every hour. What is fingerprinted in their
place is the grant's generation, which moves when a person pastes tokens and
stays put when a worker rotates them.
*/
type MCPCredentialSummary struct {
	HeaderNames []string
	EnvNames    []string
	OAuth       *MCPOAuthClient
}

// MCPOAuthClient is the part of a grant that says whose client it is.
type MCPOAuthClient struct {
	// TokenURL has any credential somebody put in the address taken out.
	TokenURL  string
	ClientID  string
	TokenType string
	Scopes    []string
}

// Summary is what a read may say about these credentials.
func (c MCPCredentials) Summary() MCPCredentialSummary {
	out := MCPCredentialSummary{HeaderNames: sortedKeys(c.Headers), EnvNames: sortedKeys(c.Env)}
	if c.OAuth != nil && (c.OAuth.TokenURL != "" || c.OAuth.ClientID != "" ||
		c.OAuth.TokenType != "" || len(c.OAuth.Scopes) > 0) {
		out.OAuth = &MCPOAuthClient{
			TokenURL: addressWithoutCredentials(c.OAuth.TokenURL), ClientID: c.OAuth.ClientID,
			TokenType: c.OAuth.TokenType, Scopes: slices.Clone(c.OAuth.Scopes),
		}
	}
	return out
}

// FingerprintMaterial is what a fingerprint of these credentials covers, or empty
// when there is none of it. The address is here whole, because what Summary takes
// out of it is exactly what would otherwise change unseen.
func (c MCPCredentials) FingerprintMaterial() []byte {
	material := struct {
		Token        string            `json:"token,omitempty"`
		Headers      map[string]string `json:"headers,omitempty"`
		Env          map[string]string `json:"env,omitempty"`
		ConfigFile   string            `json:"configFile,omitempty"`
		ClientSecret string            `json:"clientSecret,omitempty"`
		TokenURL     string            `json:"tokenURL,omitempty"`
		PrivateKey   string            `json:"privateKey,omitempty"`
		Generation   string            `json:"oauthGeneration,omitempty"`
	}{Token: c.Token, Headers: c.Headers, Env: c.Env, ConfigFile: c.ConfigFile}
	if c.OAuth != nil {
		material.ClientSecret, material.TokenURL = c.OAuth.ClientSecret, c.OAuth.TokenURL
		material.Generation = c.OAuth.Generation
	}
	if c.GitHubApp != nil {
		material.PrivateKey = c.GitHubApp.PrivateKey
	}
	raw, err := json.Marshal(material)
	if err != nil || string(raw) == "{}" {
		return nil
	}
	return raw
}

func sortedKeys(in map[string]string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// addressWithoutCredentials leaves the scheme, host and path. An address that does
// not parse is not shown at all, since what it holds cannot be told apart.
func addressWithoutCredentials(address string) string {
	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}
	parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
	return parsed.String()
}
