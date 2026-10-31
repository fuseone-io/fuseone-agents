package domain_test

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

const fakeKey = "-----BEGIN RSA PRIVATE KEY-----\nMIIfake\n-----END RSA PRIVATE KEY-----\n"

func app() *domain.MCPGitHubApp {
	return &domain.MCPGitHubApp{
		AppID: 5550564, InstallationID: 77, PrivateKey: fakeKey,
		Repositories: []string{"fuseone-agents"},
	}
}

// The App credential is sealed with the rest of the document and read back
// whole: the worker mints from what the vault gave it and nothing else.
func TestReadMCPCredentials_aGitHubApp_roundTripsThroughTheSealedDocument(t *testing.T) {
	t.Parallel()

	got := domain.ReadMCPCredentials(domain.MCPCredentials{GitHubApp: app()}.Sealed())

	if got.GitHubApp == nil || !got.GitHubApp.Equal(*app()) {
		t.Fatalf("read back %+v, want the App credential that was sealed", got.GitHubApp)
	}
}

// Switching to an App replaces a bearer, headers and an OAuth grant, and
// switching to any of those replaces an App, so a mode change never leaves
// live material nobody sees.
func TestMCPCredentialPatch_aGitHubApp_isTheOnlyRemoteCredentialLeft(t *testing.T) {
	t.Parallel()

	stored := domain.MCPCredentials{
		Token: "ghp_old", Headers: map[string]string{"Api-Key": "k"},
		OAuth: &domain.MCPOAuthGrant{AccessToken: "a"},
	}
	got := domain.MCPCredentialPatch{GitHubApp: app()}.Apply(stored)

	if got.Token != "" || got.Headers != nil || got.OAuth != nil || got.GitHubApp == nil {
		t.Fatalf("after an App write = %+v, want only the App left", got)
	}
}

func TestMCPCredentialPatch_anotherRemoteCredential_replacesTheGitHubApp(t *testing.T) {
	t.Parallel()

	token := "ghp_new"
	cases := map[string]domain.MCPCredentialPatch{
		"bearer":  {Token: &token},
		"headers": {Headers: map[string]string{"Api-Key": "k"}},
		"oauth":   {OAuth: &domain.MCPOAuthGrant{AccessToken: "a"}},
	}
	for name, patch := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := patch.Apply(domain.MCPCredentials{GitHubApp: app()})
			if got.GitHubApp != nil {
				t.Fatalf("the App survived a %s write", name)
			}
		})
	}
}

// Headers written with the App are its configuration, not a rival
// credential: the hosted GitHub MCP picks toolsets from X-MCP-Toolsets, and a
// role that moves cards needs "projects", which is not in the default set.
func TestMCPCredentialPatch_headersWrittenWithTheApp_areKeptBesideIt(t *testing.T) {
	t.Parallel()

	toolsets := map[string]string{"X-MCP-Toolsets": "issues,projects"}
	got := domain.MCPCredentialPatch{GitHubApp: app(), Headers: toolsets}.Apply(
		domain.MCPCredentials{Token: "ghp_old"})

	if got.GitHubApp == nil || got.Headers["X-MCP-Toolsets"] != "issues,projects" || got.Token != "" {
		t.Fatalf("after App+headers = %+v, want the App and its headers, no bearer", got)
	}
}

// Correcting the App — a keyless edit — does not wipe the headers stored
// beside it: the form never has their values to send back.
func TestMCPCredentialPatch_aKeylessAppEdit_keepsTheHeadersBesideIt(t *testing.T) {
	t.Parallel()

	stored := domain.MCPCredentials{GitHubApp: app(), Headers: map[string]string{"X-MCP-Toolsets": "projects"}}
	edit := domain.MCPGitHubApp{AppID: 5550564, InstallationID: 78}
	got := domain.MCPCredentialPatch{GitHubApp: &edit}.Apply(stored)

	if got.GitHubApp == nil || got.Headers["X-MCP-Toolsets"] != "projects" {
		t.Fatalf("after a keyless edit = %+v, want the stored headers kept", got)
	}
}

// Correcting the repository list must not demand pasting the key again: a
// write that names the App without a key keeps the stored one.
func TestMCPCredentialPatch_aGitHubAppWithoutAKey_keepsTheStoredKey(t *testing.T) {
	t.Parallel()

	edit := domain.MCPGitHubApp{AppID: 5550564, InstallationID: 78}
	got := domain.MCPCredentialPatch{GitHubApp: &edit}.Apply(domain.MCPCredentials{GitHubApp: app()})

	if got.GitHubApp == nil || got.GitHubApp.PrivateKey != fakeKey || got.GitHubApp.InstallationID != 78 {
		t.Fatalf("after a keyless edit = %+v, want the new installation with the stored key", got.GitHubApp)
	}
}

func TestMCPCredentialPatch_anEmptyGitHubApp_removesIt(t *testing.T) {
	t.Parallel()

	got := domain.MCPCredentialPatch{GitHubApp: &domain.MCPGitHubApp{}}.Apply(
		domain.MCPCredentials{GitHubApp: app()})

	if !got.Empty() {
		t.Fatalf("after an empty App write = %+v, want nothing left to keep", got)
	}
}

// A program the worker starts has no address to present an App token to.
func TestForTransport_stdioDropsTheGitHubApp_httpKeepsIt(t *testing.T) {
	t.Parallel()

	creds := domain.MCPCredentials{GitHubApp: app()}
	if creds.ForTransport(domain.TransportStdio).GitHubApp != nil {
		t.Fatal("a local server kept a GitHub App credential")
	}
	if creds.ForTransport(domain.TransportHTTP).GitHubApp == nil {
		t.Fatal("a remote server lost its GitHub App credential")
	}
}

// The key cannot reach a log line by accident: every format verb and slog
// render the credential without it.
func TestMCPGitHubApp_formattingAndLogging_neverRenderTheKey(t *testing.T) {
	t.Parallel()

	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("x", "app", *app(), "ptr", app())
	rendered := []string{
		fmt.Sprintf("%v", *app()), fmt.Sprintf("%+v", *app()), fmt.Sprintf("%#v", *app()),
		fmt.Sprintf("%s", *app()), fmt.Sprintf("%v", app()), logged.String(),
	}
	for _, out := range rendered {
		if strings.Contains(out, "MIIfake") || strings.Contains(out, "PRIVATE KEY") {
			t.Fatalf("rendered the key: %s", out)
		}
		if !strings.Contains(out, "5550564") {
			t.Fatalf("rendered %q, want the App id kept for diagnosis", out)
		}
	}
}

func TestMCPGitHubApp_apiBase_defaultsToGitHubDotCom(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":                                "https://api.github.com",
		"https://ghe.example.com/api/v3/": "https://ghe.example.com/api/v3",
		"https://ghe.example.com/api/v3":  "https://ghe.example.com/api/v3",
	}
	for given, want := range cases {
		if got := (domain.MCPGitHubApp{APIBaseURL: given}).APIBase(); got != want {
			t.Errorf("APIBase(%q) = %q, want %q", given, got, want)
		}
	}
}
