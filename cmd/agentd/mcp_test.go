package main

import (
	"os"
	"slices"
	"strings"
	"testing"

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

	cmd, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}

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

	cmd, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
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

	if _, err := commandFor(t.Context(), server, domain.MCPCredentials{}); err == nil {
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

	cmd, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env: map[string]string{"GITHUB_TOKEN": "ghp_configured"},
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}

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

	cmd, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env: map[string]string{"PATH": "/opt/tools/bin"},
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}

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
