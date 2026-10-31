package admin_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

func appKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func githubServer() domain.MCPServer {
	return domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.githubcopilot.com/mcp/", Enabled: true,
	}
}

func lastEventDetail(t *testing.T, target string) string {
	t.Helper()
	var detail string
	if err := openPool(t).QueryRow(context.Background(), `
		select detail::text from admin_events
		where action = 'mcp_server.configured' and target = $1
		order by event_id desc limit 1`, target).Scan(&detail); err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	return detail
}

func assertNoKey(t *testing.T, where, text, key string) {
	t.Helper()
	for _, line := range strings.Split(key, "\n") {
		if len(line) > 16 && strings.Contains(text, line) {
			t.Fatalf("%s leaked a line of the private key: %s", where, text)
		}
	}
}

// The key is sealed and comes back only through the credential reader. A
// listing says which App and installation the server acts as — enough for
// the console to show the configuration and say "key configured" — and the
// trail records that the credential changed, never what it is.
func TestPutMCPServer_aGitHubApp_sealsTheKeyAndListsOnlyWhatIsNotSecret(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	key := appKey(t)
	app := domain.MCPGitHubApp{
		AppID: 5550600, InstallationID: 91, PrivateKey: key,
		Repositories: []string{"fuseone-agents"}, APIBaseURL: "https://ghe.example.com/api/v3",
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, githubServer(),
		domain.MCPCredentialPatch{GitHubApp: &app}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	listed := servers[0].GitHubApp
	if listed == nil || listed.AppID != 5550600 || listed.InstallationID != 91 ||
		!slices.Equal(listed.Repositories, app.Repositories) || listed.APIBaseURL != app.APIBaseURL {
		t.Fatalf("listed App = %+v, want its non-secret configuration", listed)
	}
	creds, err := i.MCPCredentials(ctx, "github")
	if err != nil || creds.GitHubApp == nil || creds.GitHubApp.PrivateKey != key {
		t.Fatalf("credential reader = %v, %v; want the sealed key back", creds.GitHubApp, err)
	}
	detail := lastEventDetail(t, "github")
	if !strings.Contains(detail, `"githubAppChanged": true`) || !strings.Contains(detail, `"hasGitHubApp": true`) {
		t.Fatalf("trail = %s, want the App change recorded", detail)
	}
	assertNoKey(t, "trail", detail, key)
}

// Moving the App to another installation keeps the key nobody re-pasted, and
// the trail still says the credential changed.
func TestPutMCPServer_aGitHubAppEditWithoutAKey_keepsTheStoredKey(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	key := appKey(t)
	first := domain.MCPGitHubApp{AppID: 5550600, InstallationID: 91, PrivateKey: key}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, githubServer(),
		domain.MCPCredentialPatch{GitHubApp: &first}); err != nil {
		t.Fatalf("first write: %v", err)
	}

	moved := domain.MCPGitHubApp{AppID: 5550600, InstallationID: 92}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, githubServer(),
		domain.MCPCredentialPatch{GitHubApp: &moved}); err != nil {
		t.Fatalf("keyless edit: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "github")
	if err != nil || creds.GitHubApp == nil || creds.GitHubApp.PrivateKey != key ||
		creds.GitHubApp.InstallationID != 92 {
		t.Fatalf("after the edit = %v, %v; want installation 92 with the stored key", creds.GitHubApp, err)
	}
	if detail := lastEventDetail(t, "github"); !strings.Contains(detail, `"githubAppChanged": true`) {
		t.Fatalf("trail = %s, want the edit recorded as a credential change", detail)
	}
}

// A paste that is not an RSA key, or an App missing its ids, is refused at
// save time with a reason that does not quote what was pasted — rather than
// stored and discovered later as an unreachable server.
func TestPutMCPServer_anUnusableGitHubApp_isRefusedWithoutEchoingIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	cases := map[string]domain.MCPGitHubApp{
		"not a key":       {AppID: 1, InstallationID: 2, PrivateKey: "ghp_pasted-the-wrong-secret"},
		"no app id":       {InstallationID: 2, PrivateKey: appKey(t)},
		"no installation": {AppID: 1, PrivateKey: appKey(t)},
		"metadata base": {
			AppID: 1, InstallationID: 2, PrivateKey: appKey(t),
			APIBaseURL: "http://169.254.169.254/latest",
		},
	}
	for name, app := range cases {
		err := i.PutMCPServer(ctx, "usr_ana", platform, githubServer(),
			domain.MCPCredentialPatch{GitHubApp: &app})
		if !errors.Is(err, admin.ErrBadGitHubApp) {
			t.Fatalf("%s: err = %v, want ErrBadGitHubApp", name, err)
		}
		if strings.Contains(err.Error(), "ghp_pasted") {
			t.Fatalf("%s: the refusal echoed the paste: %v", name, err)
		}
		assertNoKey(t, name, err.Error(), app.PrivateKey)
	}
}

// Clearing the App removes the credential, so the listing stops naming it.
func TestPutMCPServer_clearingTheGitHubApp_removesIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	app := domain.MCPGitHubApp{AppID: 5550600, InstallationID: 91, PrivateKey: appKey(t)}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, githubServer(),
		domain.MCPCredentialPatch{GitHubApp: &app}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, githubServer(),
		domain.MCPCredentialPatch{GitHubApp: &domain.MCPGitHubApp{}}); err != nil {
		t.Fatalf("clear: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].GitHubApp != nil || servers[0].HasSecret {
		t.Fatalf("server = %+v, want no App and no credential left", servers[0])
	}
}
