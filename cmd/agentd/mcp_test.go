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

	cmd, err := commandFor(t.Context(), domain.MCPServer{
		Name: "local", Transport: "stdio", Command: "/bin/true",
	})
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

	cmd, err := commandFor(t.Context(), domain.MCPServer{
		Name: "local", Transport: "stdio", Command: "/bin/true",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	if slices.ContainsFunc(cmd.Env, func(v string) bool {
		return strings.HasPrefix(v, "TMPDIR=")
	}) {
		t.Errorf("Env = %v, want an unset variable left unset", cmd.Env)
	}
}
