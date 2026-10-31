// Package guard is the factory's own MCP tooling: the two GitHub writes the
// software-factory pilot (NT-013 § Guardrails, layers 5 and 5a) does not trust
// to a general-purpose GitHub server.
//
//   - record_qa_verdict sets the factory/qa commit status on exactly the head
//     the QA role checked, under the QA GitHub App's identity.
//   - merge_if_ready re-reads GitHub when it executes, after a person approved,
//     and merges only if every signal is on that head and came from the actor
//     or app that is allowed to produce it.
//
// It is tooling for this repository, not part of the platform: nothing under
// internal/ imports it, and it ships as its own binary (factory/mcp).
package guard

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrConfig marks a configuration the server refuses to start with.
var ErrConfig = errors.New("factory guard: configuration")

// Role picks the one tool an instance exposes. Each role registers its own
// instance, so no role holds the other's tool.
type Role string

const (
	RoleQA     Role = "qa"
	RoleMerger Role = "merger"
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
	Role        Role
	APIURL      string
	WebURL      string
	Repo        Repo
	Base        string
	Reviewers   []string // logins whose approval counts; the ruleset's list (#207)
	QABotID     int64    // user id of the QA App's bot, the creator of a valid factory/qa
	Checks      []string // check-run names required on the head (merger)
	ChecksAppID int64    // app that must have emitted them (merger)
	MergeMethod string
	App         AppCredentials
}

// LoadConfig reads the configuration from the environment. readFile is
// injected so tests need no files on disk.
func LoadConfig(getenv func(string) string, readFile func(string) ([]byte, error)) (Config, error) {
	e := envReader{get: getenv}
	cfg := Config{
		Role:        Role(e.required("FACTORY_ROLE")),
		APIURL:      e.orDefault("GITHUB_API_URL", "https://api.github.com"),
		WebURL:      e.orDefault("GITHUB_WEB_URL", "https://github.com"),
		Repo:        e.repo("FACTORY_REPOSITORY"),
		Base:        e.required("FACTORY_BASE_BRANCH"),
		QABotID:     e.id("FACTORY_QA_BOT_ID"),
		MergeMethod: e.orDefault("FACTORY_MERGE_METHOD", "squash"),
	}
	if cfg.Role != RoleQA && cfg.Role != RoleMerger && e.err == nil {
		e.fail("FACTORY_ROLE", "must be qa or merger")
	}
	if cfg.Role == RoleMerger {
		cfg.Reviewers = e.list("FACTORY_REVIEWERS")
		cfg.Checks = e.list("FACTORY_CHECKS")
		cfg.ChecksAppID = e.id("FACTORY_CHECKS_APP_ID")
		e.oneOf("FACTORY_MERGE_METHOD", cfg.MergeMethod, "squash", "merge", "rebase")
	}
	cfg.App = e.app(readFile)
	if e.err != nil {
		return Config{}, e.err
	}
	return cfg, nil
}

// envReader keeps the first error, so LoadConfig reads as a list of fields.
type envReader struct {
	get func(string) string
	err error
}

func (e *envReader) fail(name, why string) {
	if e.err == nil {
		e.err = fmt.Errorf("%w: %s %s", ErrConfig, name, why)
	}
}

func (e *envReader) required(name string) string {
	v := strings.TrimSpace(e.get(name))
	if v == "" {
		e.fail(name, "is required")
	}
	return v
}

func (e *envReader) orDefault(name, def string) string {
	if v := strings.TrimSpace(e.get(name)); v != "" {
		return v
	}
	return def
}

func (e *envReader) id(name string) int64 {
	raw := e.required(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		e.fail(name, "must be a positive integer")
		return 0
	}
	return n
}

func (e *envReader) list(name string) []string {
	var out []string
	for _, item := range strings.Split(e.required(name), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		e.fail(name, "must name at least one entry")
	}
	return out
}

func (e *envReader) repo(name string) Repo {
	owner, repo, ok := strings.Cut(e.required(name), "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		e.fail(name, "must be owner/name")
		return Repo{}
	}
	return Repo{Owner: owner, Name: repo}
}

func (e *envReader) oneOf(name, value string, allowed ...string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	e.fail(name, "must be one of "+strings.Join(allowed, ", "))
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
