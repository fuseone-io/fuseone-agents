package domain

import (
	"encoding/json"
	"maps"
	"slices"
	"sort"
)

/*
What a tool server is given, sealed as one document.

Three things that are not interchangeable, kept apart for the reason the two
transports are kept apart. Token is a bearer this installation *sends* to a
remote address. Env is a set of variables handed to a program it *starts*.
ConfigFile is content the worker writes to a temporary file and then names by
path to that same local program.

Sealed together because a setting holds one secret. Split across two, an
installation could exist holding a token for a server it starts and variables
for one it calls — a shape that means nothing and that nothing would refuse.

The whole document is a credential. Env and config files are not "settings":
the reason a server needs either is often that it contains a key, a DSN or a
service-account grant, and a field that is *sometimes* a secret has to be
stored as though it always is.
*/
type MCPCredentials struct {
	// Token is the bearer sent to a remote server. Meaningless for stdio,
	// which starts a program rather than calling an address.
	Token string `json:"token,omitempty"`
	/*
		OAuth is a grant for a remote HTTP server.

		It is not a dressed-up bearer. A bearer token is already the thing to
		send; an OAuth grant also carries how a worker may get a fresh access
		token. Kept apart so a screen can say which kind of credential exists
		without leaking either, and so switching mode drops the other one
		instead of leaving live material nobody sees.
	*/
	OAuth *MCPOAuthGrant `json:"oauth,omitempty"`
	/*
		Env is what a local server is given, and nothing else is.

		It is added to the small allowlist the worker copies through, and it
		wins where the two collide: a server told to use a particular PATH
		means it. What it never does is reopen inheritance — a variable absent
		from both the allowlist and this map does not reach the child, however
		much the worker holds it.
	*/
	Env map[string]string `json:"env,omitempty"`
	/*
		ConfigFile is configuration content the platform writes to a temporary
		file for a local server and then passes by path.

		It is sealed with credentials because it commonly contains DSNs, service
		account material or connection profiles. Even when it does not, a field
		that sometimes carries secrets has to be stored as though it always does.
	*/
	ConfigFile string `json:"configFile,omitempty"`
}

// MCPOAuthGrant is the remote credential an MCP server receives through the
// HTTP client that reaches it. ExpiresAtUnix is a Unix second so the sealed
// document stays stable and language-neutral.
type MCPOAuthGrant struct {
	AccessToken   string   `json:"accessToken,omitempty"`
	RefreshToken  string   `json:"refreshToken,omitempty"`
	TokenURL      string   `json:"tokenURL,omitempty"`
	ClientID      string   `json:"clientID,omitempty"`
	ClientSecret  string   `json:"clientSecret,omitempty"`
	TokenType     string   `json:"tokenType,omitempty"`
	ExpiresAtUnix int64    `json:"expiresAtUnix,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
}

func (g MCPOAuthGrant) Empty() bool {
	return g.AccessToken == "" &&
		g.RefreshToken == "" &&
		g.TokenURL == "" &&
		g.ClientID == "" &&
		g.ClientSecret == "" &&
		g.TokenType == "" &&
		g.ExpiresAtUnix == 0 &&
		len(g.Scopes) == 0
}

func (g MCPOAuthGrant) CanRefresh() bool {
	return g.RefreshToken != "" && g.TokenURL != ""
}

func (g MCPOAuthGrant) AuthorizationScheme() string {
	if g.TokenType != "" {
		return g.TokenType
	}
	return "Bearer"
}

func (g MCPOAuthGrant) Equal(other MCPOAuthGrant) bool {
	return g.AccessToken == other.AccessToken &&
		g.RefreshToken == other.RefreshToken &&
		g.TokenURL == other.TokenURL &&
		g.ClientID == other.ClientID &&
		g.ClientSecret == other.ClientSecret &&
		g.TokenType == other.TokenType &&
		g.ExpiresAtUnix == other.ExpiresAtUnix &&
		slices.Equal(g.Scopes, other.Scopes)
}

func cloneOAuth(g *MCPOAuthGrant) *MCPOAuthGrant {
	if g == nil || g.Empty() {
		return nil
	}
	out := *g
	out.Scopes = append([]string(nil), g.Scopes...)
	return &out
}

// Sealed renders the document for the vault, or empty when there is nothing to
// keep. Empty matters: it is how a write that omits credentials says "leave
// what is stored" rather than "clear it".
func (c MCPCredentials) Sealed() string {
	if c.Empty() {
		return ""
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(raw)
}

/*
ReadMCPCredentials reads what the vault gave back.

A value that is not a document is the bearer token on its own. That is what
every installation configured before this existed holds, and refusing to read
one would take a working server away in order to add a field it does not use.
*/
func ReadMCPCredentials(sealed string) MCPCredentials {
	if sealed == "" {
		return MCPCredentials{}
	}
	var c MCPCredentials
	if err := json.Unmarshal([]byte(sealed), &c); err != nil || c.Empty() {
		return MCPCredentials{Token: sealed}
	}
	c.OAuth = cloneOAuth(c.OAuth)
	return c
}

/*
MCPCredentialPatch is what one write says about a server's credentials,
including saying nothing.

Presence and value are different facts and a string cannot hold both. An
omitted token has to mean "keep what is stored" — re-entering a key to correct
an address is how people end up pasting credentials into chat to look them up —
and with only that rule a token could be written and never taken back. So the
token is a pointer: nil is silence, and empty is somebody removing it.

The map needs no pointer, because Go already tells the two apart: nil is
absent, and an empty non-nil map is a removal.
*/
type MCPCredentialPatch struct {
	Token      *string
	OAuth      *MCPOAuthGrant
	Env        map[string]string
	ConfigFile *string
}

// Apply folds a write onto what is stored.
func (p MCPCredentialPatch) Apply(stored MCPCredentials) MCPCredentials {
	out := stored
	if p.Token != nil {
		out.Token = *p.Token
		if *p.Token != "" {
			out.OAuth = nil
		}
	}
	if p.OAuth != nil {
		out.OAuth = cloneOAuth(p.OAuth)
		if out.OAuth != nil {
			out.Token = ""
		}
	}
	if p.Env != nil {
		out.Env = maps.Clone(p.Env)
	}
	if p.ConfigFile != nil {
		out.ConfigFile = *p.ConfigFile
	}
	return out
}

// Empty reports that nothing is left to keep — which is a removal to be
// carried out, not a write to be skipped.
func (c MCPCredentials) Empty() bool {
	return c.Token == "" && (c.OAuth == nil || c.OAuth.Empty()) &&
		len(c.Env) == 0 && c.ConfigFile == ""
}

/*
ForTransport drops the half this shape cannot use.

A bearer token belongs to an address, and a program the worker starts has no
address to be a bearer for. Variables belong to a process, and there is no
process when the platform is calling a URL.

Dropped rather than kept quietly. A server switched from http to stdio would
otherwise leave a live token sealed in the vault for a shape that can never
send it: material nobody can see, nobody can use, and nobody remembers to
revoke. The cost is re-entering a credential after switching back, which is
visible and asks for itself.
*/
func (c MCPCredentials) ForTransport(transport string) MCPCredentials {
	if transport == TransportStdio {
		return MCPCredentials{Env: maps.Clone(c.Env), ConfigFile: c.ConfigFile}
	}
	return MCPCredentials{Token: c.Token, OAuth: cloneOAuth(c.OAuth)}
}

/*
Environ renders the variables for a child process, sorted.

Sorted because two processes building the same environment must build the same
one: a map's order is deliberately random in Go, and an environment that
reshuffles turns a digest, a log line or a test into something that disagrees
with itself for no reason.
*/
func (c MCPCredentials) Environ() []string {
	names := make([]string, 0, len(c.Env))
	for name := range c.Env {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, name+"="+c.Env[name])
	}
	return out
}
