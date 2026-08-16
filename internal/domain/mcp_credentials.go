package domain

import (
	"encoding/json"
	"maps"
	"sort"
)

/*
What a tool server is given, sealed as one document.

Two things that are not interchangeable, kept apart for the reason the two
transports are kept apart. Token is a bearer this installation *sends* to a
remote address. Env is a set of variables handed to a program it *starts*, and
it exists only because a local server was left with no way to receive a
credential once the worker stopped handing over its own environment.

Sealed together because a setting holds one secret. Split across two, an
installation could exist holding a token for a server it starts and variables
for one it calls — a shape that means nothing and that nothing would refuse.

The whole document is a credential. Env is not configuration: the reason a
server needs a variable is almost always that the variable is a key, and a
field that is *sometimes* a secret has to be stored as though it always is.
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
}

// Sealed renders the document for the vault, or empty when there is nothing to
// keep. Empty matters: it is how a write that omits credentials says "leave
// what is stored" rather than "clear it".
func (c MCPCredentials) Sealed() string {
	if len(c.Env) == 0 && c.Token == "" {
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
	if err := json.Unmarshal([]byte(sealed), &c); err != nil || (c.Token == "" && len(c.Env) == 0) {
		return MCPCredentials{Token: sealed}
	}
	return c
}

/*
Merge keeps whichever half a write left out.

Correcting an address must not demand re-entering a token nobody has to hand,
and adding a variable must not silently drop one. Absent means unchanged; the
way to remove something is to send it empty, which is a different request from
not sending it.
*/
func (c MCPCredentials) Merge(given MCPCredentials) MCPCredentials {
	merged := MCPCredentials{Token: c.Token}
	if given.Token != "" {
		merged.Token = given.Token
	}
	if given.Env == nil {
		merged.Env = c.Env
		return merged
	}
	merged.Env = maps.Clone(given.Env)
	return merged
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
