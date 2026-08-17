// Package known is what the platform already knows about tool servers other
// people publish.
//
// Registering an MCP server discovers its tools and files every one of them
// unclassified, because nothing verifies what a server says about itself
// (PRD DE-12, DE-13). That is correct, and it is also forty rulings to write
// by hand from a list of bare names before a well-known server does anything —
// which is how a safe default becomes one people work around.
//
// So the platform ships what it knows: a suggested effect per tool, a suggested
// compensator, and the sentence a Curator reads before confirming. **A
// suggestion never becomes a classification on its own.** Applied on import it
// would put the decision back in a table shipped in a binary, which is the same
// mistake as trusting the server, one step further away and harder to see.
package known

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed servers/*.yaml
var files embed.FS

// Provenance says how far an entry should be trusted.
//
// On the entry rather than in a comment because it changes what the console
// tells the Curator. A suggestion checked against a running server is a
// different thing from one read off a documentation page, and presenting them
// alike makes the careful ones worth less rather than the careless ones worth
// more.
type Provenance string

const (
	// FromServer means somebody ran this server and these are the names it
	// answered with.
	FromServer Provenance = "server"
	// FromDocumentation means the entry was written from what the publisher
	// documents. Names may be stale, and one that does not match is simply not
	// suggested.
	FromDocumentation Provenance = "documentation"
)

// Status says what kind of recipe this is.
type Status string

const (
	StatusPublished Status = "published"
	StatusReference Status = "reference"
	StatusArchived  Status = "archived"
)

// ConfigRequirement says what an operator must bring for this recipe to run.
type ConfigRequirement string

const (
	ConfigCredential ConfigRequirement = "credential"
	ConfigEnv        ConfigRequirement = "env"
	ConfigFile       ConfigRequirement = "file"
	ConfigPath       ConfigRequirement = "path"
)

// AuthType says how a credential is presented to the server.
type AuthType string

const (
	AuthNone       AuthType = "none"
	AuthOAuth2     AuthType = "oauth2"
	AuthBearer     AuthType = "bearer"
	AuthBasic      AuthType = "basic"
	AuthHeaders    AuthType = "headers"
	AuthEnv        AuthType = "env"
	AuthConfigFile AuthType = "config_file"
	AuthPath       AuthType = "path"
	AuthDSN        AuthType = "dsn"
)

// AuthPrincipal says whose authority the credential carries.
type AuthPrincipal string

const (
	AuthPrincipalNone         AuthPrincipal = "none"
	AuthPrincipalUser         AuthPrincipal = "user"
	AuthPrincipalInstallation AuthPrincipal = "installation"
	AuthPrincipalService      AuthPrincipal = "service"
)

// AuthMode is one authentication shape documented for a recipe.
type AuthMode struct {
	Type      AuthType      `yaml:"type"`
	Principal AuthPrincipal `yaml:"principal"`
	Label     string        `yaml:"label,omitempty"`
	Header    string        `yaml:"header,omitempty"`
	Prefix    string        `yaml:"prefix,omitempty"`
	Scopes    []string      `yaml:"scopes,omitempty"`
	Note      string        `yaml:"note,omitempty"`
}

// Suggestion is what the platform believes one tool does.
type Suggestion struct {
	Tool string `yaml:"tool"`
	// Effect is a domain.Effect by name, kept as a string so this package
	// stays data.
	Effect string `yaml:"effect"`
	// Untrusted marks a source whose output may be attacker-authored. Absent
	// means no opinion, and the safe default applies.
	Untrusted *bool `yaml:"untrusted,omitempty"`
	// CompensatedBy is the tool that takes this one back, if any.
	CompensatedBy string `yaml:"compensatedBy,omitempty"`
	// Why is what the Curator reads before confirming. Required: a suggested
	// classification with no reasoning is a number to click past.
	Why string `yaml:"why"`
}

/*
DocsSource says whose documentation an entry links to.

Separate from Provenance because they are different questions and collapsing
them is how a catalogue starts lying. Provenance is where the *suggestions*
came from — somebody ran the server, or somebody read a page. This is whose
page. A community server that was actually run has trustworthy suggestions and
no publisher standing behind it; a publisher's own server read off their site
is the reverse.

Neither is "official", and the console must never render that word. It is the
one label a reader takes as a promise of support, and no entry here is one:
this platform did not write these servers, does not host them, and vouches for
nothing beyond saying where it read what it read.
*/
type DocsSource string

const (
	// DocsFromPublisher means Docs points at the documentation of whoever
	// publishes the server.
	DocsFromPublisher DocsSource = "publisher"
	// DocsFromThirdParty means somebody else wrote it down — a directory, a
	// write-up, a fork's README. Useful and further from the source.
	DocsFromThirdParty DocsSource = "third-party"
)

/*
Categories are the shelves, and the list is closed.

Open, every entry invents its own and the rail becomes as long as the
catalogue. Coarse, because a shelf people argue about is one they stop using —
these say what a server is *for*, which is the question somebody browsing is
actually asking.
*/
var Categories = []string{
	"code", "data", "finance", "knowledge", "communication", "operations", "web",
}

// Entry is one server the platform knows about.
type Entry struct {
	// Server is the local name this applies to. Matching is by the name the
	// Curator registered, because it is the only thing both sides share.
	Server    string `yaml:"server"`
	Title     string `yaml:"title"`
	Publisher string `yaml:"publisher"`
	/*
		Category is the shelf it sits on, for a catalogue somebody browses.

		A flat list of every server anybody publishes is a list nobody reads to
		the end. The shelves are coarse on purpose — what the thing is for, not
		who sells it — because a taxonomy fine enough to be interesting is one
		nobody agrees with.
	*/
	Category   string     `yaml:"category"`
	Docs       string     `yaml:"docs"`
	DocsFrom   DocsSource `yaml:"docsFrom"`
	Provenance Provenance `yaml:"provenance"`
	// Status tells the console whether this is a current published server, a
	// reference implementation, or a recipe whose source says it is archived.
	// Required: "published" is an assertion, not the zero value.
	Status Status `yaml:"status"`
	// Config is the shape of what the operator must bring: a credential, env
	// vars, a config file, or a path the worker can see.
	Config []ConfigRequirement `yaml:"config,omitempty"`

	/*
		What the recipe proposes for the connection form.

		Filled in, never submitted. A suggested command is a program somebody
		is one click from running inside the worker, which is exactly why the
		acceptance stays a separate act — this saves the typing and decides
		nothing.

		Empty transport is allowed and often honest: several servers ship both
		a container and a hosted endpoint, and choosing for somebody is a
		recommendation this package has no basis for.
	*/
	Transport string   `yaml:"transport,omitempty"`
	Command   string   `yaml:"command,omitempty"`
	Args      []string `yaml:"args,omitempty"`
	URL       string   `yaml:"url,omitempty"`
	// Auth is the credential it expects, in words a person reads before going
	// to fetch one. Not a field name and not a schema: what to get, and what
	// it will be able to reach.
	Auth string `yaml:"auth,omitempty"`
	/*
		AuthModes are the structured facts behind Auth.

		The connection runtime does not implement every one of these today. That
		is exactly why the field exists: a recipe that needs OAuth, Basic auth,
		a DSN or a generated config file must not be flattened into "paste a
		token here" merely because the first runtime path happens to know bearer
		tokens.
	*/
	AuthModes []AuthMode `yaml:"authModes,omitempty"`
	// Note is what an operator has to know before running it at all — usually
	// the credential it wants and what that credential can reach.
	Note        string       `yaml:"note,omitempty"`
	Suggestions []Suggestion `yaml:"suggestions"`
}

// Servers is every entry the platform ships.
type Servers struct{ entries map[string]Entry }

// Load reads the shipped entries.
func Load() (*Servers, error) {
	return load(files)
}

func load(fsys fs.FS) (*Servers, error) {
	k := &Servers{entries: map[string]Entry{}}

	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		raw, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("known: read %s: %w", path, err)
		}
		var entry Entry
		if err := yaml.Unmarshal(raw, &entry); err != nil {
			return fmt.Errorf("known: parse %s: %w", path, err)
		}
		if err := check(path, entry); err != nil {
			return err
		}
		if _, exists := k.entries[entry.Server]; exists {
			return fmt.Errorf("known: %s duplicates server %q", path, entry.Server)
		}
		k.entries[entry.Server] = entry
		return nil
	})
	if err != nil {
		return nil, err
	}
	return k, nil
}

/*
check refuses a recipe that would mislead rather than help.

Shipped data is still data somebody wrote by hand, and every one of these is a
sentence the console shows a Curator as though the platform stood behind it.
The failures worth catching at build time are the ones that read as confidence:
a link to nowhere presented as the publisher's own, a transport the form cannot
hold, a suggestion with no reasoning behind it.
*/
func check(path string, entry Entry) error {
	switch {
	case entry.Server == "":
		return fmt.Errorf("known: %s names no server", path)
	case entry.Title == "" || entry.Publisher == "":
		return fmt.Errorf("known: %s does not say what it is or who publishes it", path)
	case !slices.Contains(Categories, entry.Category):
		return fmt.Errorf("known: %s is on no shelf (%q)", path, entry.Category)
	case entry.DocsFrom == "":
		return fmt.Errorf("known: %s does not say whose documentation it points at", path)
	case entry.DocsFrom == DocsFromPublisher && entry.Docs == "":
		return fmt.Errorf("known: %s claims the publisher's documentation and links to none", path)
	case entry.Status == "":
		return fmt.Errorf("known: %s does not say whether the recipe is published, reference or archived", path)
	case entry.Status != StatusPublished && entry.Status != StatusReference && entry.Status != StatusArchived:
		return fmt.Errorf("known: %s has unknown status %q", path, entry.Status)
	case entry.Transport == "stdio" && entry.Command == "":
		return fmt.Errorf("known: %s suggests stdio and no command", path)
	case entry.Transport == "http" && entry.URL == "":
		return fmt.Errorf("known: %s suggests http and no address", path)
	case entry.Transport != "" && entry.Transport != "stdio" && entry.Transport != "http":
		return fmt.Errorf("known: %s suggests %q, which is not a transport", path, entry.Transport)
	}
	for _, one := range entry.Suggestions {
		if one.Why == "" {
			return fmt.Errorf("known: %s suggests %s with no reasoning", path, one.Tool)
		}
	}
	for _, one := range entry.Config {
		switch one {
		case ConfigCredential, ConfigEnv, ConfigFile, ConfigPath:
		default:
			return fmt.Errorf("known: %s asks for unknown configuration %q", path, one)
		}
	}
	if err := checkAuthModes(path, entry); err != nil {
		return err
	}
	return nil
}

func checkAuthModes(path string, entry Entry) error {
	for _, one := range entry.AuthModes {
		switch one.Type {
		case AuthNone, AuthOAuth2, AuthBearer, AuthBasic, AuthHeaders, AuthEnv, AuthConfigFile, AuthPath, AuthDSN:
		default:
			return fmt.Errorf("known: %s names unknown auth type %q", path, one.Type)
		}
		switch one.Principal {
		case AuthPrincipalNone, AuthPrincipalUser, AuthPrincipalInstallation, AuthPrincipalService:
		default:
			return fmt.Errorf("known: %s names unknown auth principal %q", path, one.Principal)
		}
		if one.Type == AuthNone && one.Principal != AuthPrincipalNone {
			return fmt.Errorf("known: %s says auth is none but gives principal %q", path, one.Principal)
		}
	}

	hasCredential := slices.Contains(entry.Config, ConfigCredential)
	if !hasCredential {
		return nil
	}
	if len(entry.AuthModes) == 0 {
		return fmt.Errorf("known: %s asks for a credential but does not say what kind", path)
	}
	for _, one := range entry.AuthModes {
		switch one.Type {
		case AuthOAuth2, AuthBearer, AuthBasic, AuthHeaders, AuthConfigFile, AuthDSN:
			return nil
		}
	}
	return fmt.Errorf("known: %s asks for a credential but only names non-credential auth modes", path)
}

/*
Suggest answers what the platform believes about one discovered tool.

Matched against the name the server actually answered with. A suggestion for a
tool the server no longer offers is not returned, and a tool the entry never
heard of gets nothing — so an entry that has aged degrades into silence rather
than into a wrong answer. That is the property that makes shipping these
defensible: the worst a stale entry can do is leave the Curator exactly where
they would have been without it.
*/
func (s *Servers) Suggest(server, remoteName string) (Suggestion, bool) {
	entry, ok := s.entries[server]
	if !ok {
		return Suggestion{}, false
	}
	for _, one := range entry.Suggestions {
		if one.Tool == remoteName {
			return one, true
		}
	}
	return Suggestion{}, false
}

// For answers what is known about a server, or nothing.
func (s *Servers) For(server string) (Entry, bool) {
	entry, ok := s.entries[server]
	return entry, ok
}

// All lists what is known, for the registration screen.
func (s *Servers) All() []Entry {
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	return out
}
