package guard

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// mergerEnv is a complete Merger configuration. Each test removes or changes
// one variable, so a failure names exactly the variable it is about.
func mergerEnv() map[string]string {
	return map[string]string{
		"FACTORY_ROLE":                "merger",
		"FACTORY_REPOSITORY":          "fuseone-io/fuseone-agents",
		"FACTORY_BASE_BRANCH":         "main",
		"FACTORY_REVIEWERS":           "fuseone-reviewer[bot], Maintainer-One",
		"FACTORY_QA_BOT_ID":           "9001",
		"FACTORY_CHECKS":              "go,postgres,console,chart,artifact",
		"FACTORY_CHECKS_APP_ID":       "15368",
		"GITHUB_APP_ID":               "123",
		"GITHUB_APP_INSTALLATION_ID":  "456",
		"GITHUB_APP_PRIVATE_KEY_FILE": "/run/secrets/key.pem",
	}
}

func qaEnv() map[string]string {
	return map[string]string{
		"FACTORY_ROLE":                "qa",
		"FACTORY_REPOSITORY":          "fuseone-io/fuseone-agents",
		"FACTORY_BASE_BRANCH":         "main",
		"FACTORY_QA_BOT_ID":           "9001",
		"GITHUB_APP_ID":               "777",
		"GITHUB_APP_INSTALLATION_ID":  "888",
		"GITHUB_APP_PRIVATE_KEY_FILE": "/run/secrets/key.pem",
	}
}

func load(env map[string]string) (Config, error) {
	return LoadConfig(
		func(k string) string { return env[k] },
		func(path string) ([]byte, error) {
			if path == "/run/secrets/key.pem" {
				return []byte("PEM"), nil
			}
			return nil, os.ErrNotExist
		})
}

func TestLoadConfig_completeMergerEnvironment_parsesEveryField(t *testing.T) {
	t.Parallel()

	cfg, err := load(mergerEnv())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Role != RoleMerger || cfg.Repo.FullName() != "fuseone-io/fuseone-agents" || cfg.Base != "main" {
		t.Fatalf("role/repo/base = %q %q %q", cfg.Role, cfg.Repo.FullName(), cfg.Base)
	}
	if got := strings.Join(cfg.Reviewers, "|"); got != "fuseone-reviewer[bot]|Maintainer-One" {
		t.Fatalf("reviewers = %q", got)
	}
	if cfg.QABotID != 9001 || cfg.ChecksAppID != 15368 || len(cfg.Checks) != 5 {
		t.Fatalf("qa bot %d, checks app %d, checks %v", cfg.QABotID, cfg.ChecksAppID, cfg.Checks)
	}
	if cfg.App.AppID != 123 || cfg.App.InstallationID != 456 || string(cfg.App.PrivateKeyPEM) != "PEM" {
		t.Fatalf("app = %+v", cfg.App)
	}
	if cfg.APIURL != "https://api.github.com" || cfg.MergeMethod != "squash" {
		t.Fatalf("defaults: api %q, method %q", cfg.APIURL, cfg.MergeMethod)
	}
}

// Nothing the guard decides with has a default: an unset reviewer list, check
// list or QA identity must stop the server, never fall back to something that
// lets a merge through.
func TestLoadConfig_missingVariable_refusedNamingIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		env  func() map[string]string
		drop string
	}{
		{mergerEnv, "FACTORY_ROLE"},
		{mergerEnv, "FACTORY_REPOSITORY"},
		{mergerEnv, "FACTORY_BASE_BRANCH"},
		{mergerEnv, "FACTORY_REVIEWERS"},
		{mergerEnv, "FACTORY_QA_BOT_ID"},
		{mergerEnv, "FACTORY_CHECKS"},
		{mergerEnv, "FACTORY_CHECKS_APP_ID"},
		{mergerEnv, "GITHUB_APP_ID"},
		{mergerEnv, "GITHUB_APP_INSTALLATION_ID"},
		{mergerEnv, "GITHUB_APP_PRIVATE_KEY_FILE"},
		{qaEnv, "FACTORY_QA_BOT_ID"},
		{qaEnv, "GITHUB_APP_ID"},
	}
	for _, tc := range cases {
		t.Run(tc.drop, func(t *testing.T) {
			t.Parallel()
			env := tc.env()
			delete(env, tc.drop)
			_, err := load(env)
			if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), tc.drop) {
				t.Fatalf("err = %v, want ErrConfig naming %s", err, tc.drop)
			}
		})
	}
}

func TestLoadConfig_qaRole_needsNoMergerSettings(t *testing.T) {
	t.Parallel()

	cfg, err := load(qaEnv())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Role != RoleQA || cfg.QABotID != 9001 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadConfig_invalidValue_refused(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"FACTORY_ROLE":          "reviewer",
		"FACTORY_REPOSITORY":    "fuseone-agents",
		"FACTORY_QA_BOT_ID":     "qa",
		"FACTORY_CHECKS_APP_ID": "-1",
		"FACTORY_MERGE_METHOD":  "fast-forward",
		"FACTORY_REVIEWERS":     " , ",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := mergerEnv()
			env[name] = value
			_, err := load(env)
			if !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), name) {
				t.Fatalf("err = %v, want ErrConfig naming %s", err, name)
			}
		})
	}
}

func TestLoadConfig_inlinePrivateKey_usedWhenNoFile(t *testing.T) {
	t.Parallel()

	env := mergerEnv()
	delete(env, "GITHUB_APP_PRIVATE_KEY_FILE")
	env["GITHUB_APP_PRIVATE_KEY"] = "INLINE"
	cfg, err := load(env)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(cfg.App.PrivateKeyPEM) != "INLINE" {
		t.Fatalf("key = %q", cfg.App.PrivateKeyPEM)
	}
}

func TestLoadConfig_unreadableKeyFile_refused(t *testing.T) {
	t.Parallel()

	env := mergerEnv()
	env["GITHUB_APP_PRIVATE_KEY_FILE"] = "/nowhere.pem"
	if _, err := load(env); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want the read error wrapped", err)
	}
}
