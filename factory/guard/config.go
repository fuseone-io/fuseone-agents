// Package guard is the factory's own MCP tooling: the GitHub writes the
// software-factory pilot (NT-013 § Guardrails) does not trust to a
// general-purpose GitHub server.
//
//   - record_qa_verdict sets the factory/qa commit status on exactly the head
//     the QA role checked, under the QA GitHub App's identity.
//   - merge_if_ready re-reads GitHub when it executes, after a person approved,
//     and merges only if every signal is on that head and came from the actor
//     or app that is allowed to produce it.
//   - move_card changes only the Status of a card in the factory's project,
//     and only along the calling role's row of the transition table.
//   - set_card_fields sets only a card's Work type and Size.
//
// It is tooling for this repository, not part of the platform: nothing under
// internal/ imports it, and it ships as its own binary (factory/mcp).
package guard

import (
	"errors"
	"fmt"
	"strings"
)

// ErrConfig marks a configuration the server refuses to start with.
var ErrConfig = errors.New("factory guard: configuration")

// AuthMode is where the GitHub credential of a call comes from.
type AuthMode string

const (
	// AuthApp: the server holds one GitHub App's key and mints its own
	// tokens; FACTORY_ROLE names the one role it serves.
	AuthApp AuthMode = "app"
	// AuthPassThrough: each request's bearer is the calling role's GitHub App
	// installation token. The server holds no credential, learns the caller's
	// bot identity from the token, and serves that identity's role.
	AuthPassThrough AuthMode = "github-app-token"
)

// Repo is the single repository the guard acts on.
type Repo struct{ Owner, Name string }

// FullName is owner/name, the form GitHub reports in base.repo.full_name.
func (r Repo) FullName() string { return r.Owner + "/" + r.Name }

// AppCredentials are a GitHub App's: the guard mints its own short-lived
// installation tokens from them, so nothing long-lived sits in the process.
type AppCredentials struct {
	AppID          int64
	InstallationID int64
	PrivateKeyPEM  []byte
}

// Config is everything the guard decides with. None of the deciding fields has
// a default: an unset list must stop the server, not let a merge through.
type Config struct {
	Auth        AuthMode
	Role        Role           // AuthApp: the one role served
	RoleBots    map[int64]Role // AuthPassThrough: bot user id -> role
	APIURL      string
	GraphQLURL  string
	WebURL      string
	Repo        Repo
	Base        string
	Reviewers   []string // logins whose approval counts; the ruleset's list (#207)
	QABotID     int64    // user id of the QA App's bot, the creator of a valid factory/qa
	Checks      []string // check-run names required on the head (merger)
	ChecksAppID int64    // app that must have emitted them (merger)
	MergeMethod string
	Board       BoardConfig
	App         AppCredentials
}

// Roles is every role this configuration serves.
func (c Config) Roles() []Role {
	if c.Auth == AuthApp {
		return []Role{c.Role}
	}
	var out []Role
	for _, r := range Roles() {
		for _, served := range c.RoleBots {
			if served == r {
				out = append(out, r)
			}
		}
	}
	return out
}

func (c Config) serves(tool string) bool {
	for _, r := range c.Roles() {
		if hasTool(r, tool) {
			return true
		}
	}
	return false
}

// LoadConfig reads the configuration from the environment. A setting is
// required only when a served role uses it. readFile is injected so tests
// need no files on disk.
func LoadConfig(getenv func(string) string, readFile func(string) ([]byte, error)) (Config, error) {
	e := envReader{get: getenv}
	cfg := Config{
		APIURL:      e.orDefault("GITHUB_API_URL", "https://api.github.com"),
		GraphQLURL:  e.orDefault("GITHUB_GRAPHQL_URL", "https://api.github.com/graphql"),
		WebURL:      e.orDefault("GITHUB_WEB_URL", "https://github.com"),
		MergeMethod: e.orDefault("FACTORY_MERGE_METHOD", "squash"),
	}
	e.identity(&cfg)
	e.repository(&cfg)
	if cfg.serves(ToolMergeIfReady) {
		cfg.Reviewers = e.list("FACTORY_REVIEWERS")
		cfg.Checks = e.list("FACTORY_CHECKS")
		cfg.ChecksAppID = e.id("FACTORY_CHECKS_APP_ID")
		e.oneOf("FACTORY_MERGE_METHOD", cfg.MergeMethod, "squash", "merge", "rebase")
	}
	cfg.Board = e.board(cfg.serves(ToolSetCardFields))
	if cfg.Auth == AuthApp {
		cfg.App = e.app(readFile)
	}
	if e.err != nil {
		return Config{}, e.err
	}
	return cfg, nil
}

// identity reads who the server serves: one role under its own App, or the
// bot-to-role allowlist when each request brings its own token.
func (e *envReader) identity(cfg *Config) {
	cfg.Auth = AuthMode(e.orDefault("FACTORY_MCP_AUTH", string(AuthApp)))
	switch cfg.Auth {
	case AuthApp:
		cfg.Role = Role(e.required("FACTORY_ROLE"))
		if !knownRole(cfg.Role) {
			e.fail("FACTORY_ROLE", "must be one of "+strings.Join(names(Roles()), ", "))
		}
	case AuthPassThrough:
		e.absent("FACTORY_ROLE", "is not read with FACTORY_MCP_AUTH=github-app-token: "+
			"the role comes from the token's identity and FACTORY_ROLE_BOTS")
		cfg.RoleBots = e.roleBots("FACTORY_ROLE_BOTS")
	default:
		e.fail("FACTORY_MCP_AUTH", "must be app or github-app-token")
	}
}

// repository reads the repository and its protected base. QA and the Merger
// act on pull requests; under its own App, any role needs the repository its
// tokens are restricted to.
func (e *envReader) repository(cfg *Config) {
	prs := cfg.serves(ToolRecordQAVerdict) || cfg.serves(ToolMergeIfReady)
	if prs || cfg.Auth == AuthApp {
		cfg.Repo = e.repo("FACTORY_REPOSITORY")
	}
	if prs {
		cfg.Base = e.required("FACTORY_BASE_BRANCH")
		cfg.QABotID = e.id("FACTORY_QA_BOT_ID")
	}
}

// app reads the App credentials. The key comes from a file (a mounted secret)
// or, failing that, inline; the file is preferred because a multi-line PEM in
// an environment variable is easy to mangle and easy to leak in a dump.
func (e *envReader) app(readFile func(string) ([]byte, error)) AppCredentials {
	creds := AppCredentials{
		AppID:          e.id("GITHUB_APP_ID"),
		InstallationID: e.id("GITHUB_APP_INSTALLATION_ID"),
	}
	if path := strings.TrimSpace(e.get("GITHUB_APP_PRIVATE_KEY_FILE")); path != "" {
		pem, err := readFile(path)
		if err != nil && e.err == nil {
			e.err = fmt.Errorf("%w: GITHUB_APP_PRIVATE_KEY_FILE: %w", ErrConfig, err)
		}
		creds.PrivateKeyPEM = pem
		return creds
	}
	if inline := e.get("GITHUB_APP_PRIVATE_KEY"); inline != "" {
		creds.PrivateKeyPEM = []byte(inline)
		return creds
	}
	e.fail("GITHUB_APP_PRIVATE_KEY_FILE", "or GITHUB_APP_PRIVATE_KEY is required")
	return creds
}
