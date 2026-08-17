package main

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
What a locally executed tool server is handed.

A stdio server is not another transport. It is a program this installation
starts inside the worker, and until now it started with `cmd.Env` unset — which
in Go means the child inherits everything the worker holds. The worker holds
the database URL and the master key.

So the Gate governed what the *tool* could do while the *process* could read
the key and open the database itself, with no tool call, no ledger step and no
decision. Scrubbing the environment does not make stdio safe — it is still code
running as the worker — but it stops the platform from handing over its own
credentials to do it with.
*/
func TestCommandFor_aStdioServer_doesNotInheritTheWorkersEnvironment(t *testing.T) {
	t.Setenv("FUSEONE_MASTER_KEY", "must-not-travel")
	t.Setenv("DATABASE_URL", "postgres://must-not-travel")
	t.Setenv("PATH", "/usr/bin:/bin")

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if cmd.Env == nil {
		t.Fatal("Env is nil, which is how the child inherits everything")
	}
	for _, secret := range []string{"FUSEONE_MASTER_KEY", "DATABASE_URL"} {
		if i := slices.IndexFunc(cmd.Env, func(v string) bool {
			return strings.HasPrefix(v, secret+"=")
		}); i >= 0 {
			t.Errorf("the child is handed %s", secret)
		}
	}

	// And the other direction, or an empty Env would pass this test while
	// leaving a server unable to find the binary it was told to run.
	if !slices.Contains(cmd.Env, "PATH=/usr/bin:/bin") {
		t.Errorf("Env = %v, want the operational variables a program needs", cmd.Env)
	}
}

/*
A variable the worker does not have is not invented.

`TMPDIR=` is not the same as no TMPDIR: the first is a temporary directory that
does not exist, and a program handed one fails somewhere far from here. The
worker's environment is copied, never approximated.

Unset with os.Unsetenv rather than `t.Setenv(name, "")`, which sets it to
empty — the first version of this test did that and passed on the behaviour it
was written to forbid.
*/
func TestCommandFor_aVariableTheWorkerDoesNotHold_isNotPassedEmpty(t *testing.T) {
	t.Setenv("TMPDIR", "restored-by-cleanup")
	if err := os.Unsetenv("TMPDIR"); err != nil {
		t.Fatalf("unset: %v", err)
	}

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)
	if slices.ContainsFunc(cmd.Env, func(v string) bool {
		return strings.HasPrefix(v, "TMPDIR=")
	}) {
		t.Errorf("Env = %v, want an unset variable left unset", cmd.Env)
	}
}

// accepted is a local server somebody has explicitly agreed to run.
func accepted() domain.MCPServer {
	return domain.MCPServer{
		Name: "local", Transport: "stdio", Command: "/bin/true",
		AcceptsLocalExecution: true,
	}
}

/*
A local server nobody accepted is not started.

Refused here as well as where it is written, because a row can arrive by
restore, by migration, or from a version of the console that did not ask. The
door checking a rule the runtime does not is a rule that holds until the first
time it matters.
*/
func TestCommandFor_aLocalServerNobodyAccepted_isNotStarted(t *testing.T) {
	server := accepted()
	server.AcceptsLocalExecution = false

	if _, _, err := commandFor(t.Context(), server, domain.MCPCredentials{}); err == nil {
		t.Fatal("no error; a program nobody agreed to would have been started")
	} else if !strings.Contains(err.Error(), "local") {
		t.Errorf("err = %v, want a sentence naming what was not accepted", err)
	}
}

/*
A local server's own variables reach it, and only its own.

The allowlist closed the hole and took the capability with it: before this,
inheritance was how a local server ever got a token. So it receives them
explicitly, from the vault, per server — and the worker's own secrets still do
not travel, which is the property the whole change exists for.
*/
func TestCommandFor_theServersOwnVariables_areGivenWithoutReopeningInheritance(t *testing.T) {
	t.Setenv("FUSEONE_MASTER_KEY", "must-not-travel")
	t.Setenv("PATH", "/usr/bin:/bin")

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env: map[string]string{"GITHUB_TOKEN": "ghp_configured"},
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if !slices.Contains(cmd.Env, "GITHUB_TOKEN=ghp_configured") {
		t.Errorf("Env = %v, want the configured variable", cmd.Env)
	}
	if !slices.Contains(cmd.Env, "PATH=/usr/bin:/bin") {
		t.Errorf("Env = %v, want the allowlist as well", cmd.Env)
	}
	if slices.ContainsFunc(cmd.Env, func(v string) bool {
		return strings.HasPrefix(v, "FUSEONE_MASTER_KEY=")
	}) {
		t.Error("configuring a variable reopened inheritance")
	}
}

// A server that names a variable the allowlist also carries means the one it
// named. Its own configuration is the more specific statement.
func TestCommandFor_aConfiguredVariable_winsOverTheOneCopiedThrough(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env: map[string]string{"PATH": "/opt/tools/bin"},
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	// Both are present and the configured one is last, which is what an
	// exec environment resolves to.
	last := ""
	for _, v := range cmd.Env {
		if strings.HasPrefix(v, "PATH=") {
			last = v
		}
	}
	if last != "PATH=/opt/tools/bin" {
		t.Errorf("PATH resolves to %q, want the configured one", last)
	}
}

/*
A managed config file is content, not a path typed into args.

The platform owns the file it creates: it writes it in a private temporary
directory, hands the path to the local process by environment, and removes it
when the MCP session is closed. That keeps the operator from having to place a
secret file on the worker's filesystem by hand.
*/
func TestCommandFor_aConfigFile_isMaterializedAndNamedByEnvironment(t *testing.T) {
	server := accepted()
	cmd, cleanup, err := commandFor(t.Context(), server, domain.MCPCredentials{
		ConfigFile: "dsn: postgres://agent:secret@db/app\n",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}

	path := envValue(cmd.Env, domain.DefaultMCPConfigFileEnv)
	if path == "" {
		t.Fatalf("Env = %v, want the managed config path", cmd.Env)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized config: %v", err)
	}
	if string(body) != "dsn: postgres://agent:secret@db/app\n" {
		t.Errorf("config body = %q", body)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat materialized config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("after cleanup stat = %v, want file gone", err)
	}
}

func TestCommandFor_aConfigFile_canUseACustomEnvironmentName(t *testing.T) {
	name := "TOOLBOX_CONFIG"
	server := accepted()
	server.ConfigFileEnv = &name

	cmd, cleanup, err := commandFor(t.Context(), server, domain.MCPCredentials{
		ConfigFile: "sources: []\n",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if envValue(cmd.Env, "TOOLBOX_CONFIG") == "" {
		t.Fatalf("Env = %v, want TOOLBOX_CONFIG to name the materialized file", cmd.Env)
	}
}

func TestCommandFor_aManagedConfigPathWinsOverAHandWrittenVariable(t *testing.T) {
	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env:        map[string]string{domain.DefaultMCPConfigFileEnv: "/tmp/hand-written.yaml"},
		ConfigFile: "managed: true\n",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if got := envValue(cmd.Env, domain.DefaultMCPConfigFileEnv); got == "/tmp/hand-written.yaml" || got == "" {
		t.Fatalf("config env resolves to %q, want the managed path to win", got)
	}
}

func envValue(env []string, name string) string {
	prefix := name + "="
	out := ""
	for _, one := range env {
		if strings.HasPrefix(one, prefix) {
			out = strings.TrimPrefix(one, prefix)
		}
	}
	return out
}

/*
A rotated credential takes effect on the next pass, not the next deploy.

The reconciler keeps a session while the fingerprint holds, and the fingerprint
saw the address and not the secret — same command, same URL, same flags — so a
replaced token left the old session in use until a restart or an unrelated
edit. A credential is rotated most urgently when it has leaked, which is
exactly when "at the next deploy" is the wrong answer.

The timestamp stands in for the secret. Reading every server's credential on
every pass would unseal the vault on a timer to answer what the row answers.
*/
func TestFingerprint_aServerWrittenAgain_isNotTheOneInHand(t *testing.T) {
	t.Parallel()

	before := domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
		UpdatedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	}
	rotated := before
	rotated.UpdatedAt = before.UpdatedAt.Add(time.Minute)

	if fingerprint(before) == fingerprint(rotated) {
		t.Error("unchanged; the worker would go on using the credential that was replaced")
	}
}

// And a pass that changed nothing keeps the session. Reconnecting every server
// on every sweep would make a tool call fail whenever a pass landed on it.
func TestFingerprint_aServerNobodyTouched_keepsItsSession(t *testing.T) {
	t.Parallel()

	server := domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
		UpdatedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	}
	if fingerprint(server) != fingerprint(server) {
		t.Error("a server reconnects on every pass")
	}
}
