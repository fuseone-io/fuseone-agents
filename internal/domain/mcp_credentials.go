package domain

import (
	"encoding/json"
	"maps"
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
	Env        map[string]string
	ConfigFile *string
}

// Apply folds a write onto what is stored.
func (p MCPCredentialPatch) Apply(stored MCPCredentials) MCPCredentials {
	out := stored
	if p.Token != nil {
		out.Token = *p.Token
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
	return c.Token == "" && len(c.Env) == 0 && c.ConfigFile == ""
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
		return MCPCredentials{Env: c.Env, ConfigFile: c.ConfigFile}
	}
	return MCPCredentials{Token: c.Token}
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
