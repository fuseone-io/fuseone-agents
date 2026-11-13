package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
An area writes its own remote MCP servers (NT-019, phase 2 writers).

Stored at the area, read back at the area, and a local one refused: a stdio
server runs on the platform's worker, which no area may command (decided
2026-11-13).
*/

func remoteServer(name string) domain.MCPServer {
	return domain.MCPServer{Name: name, Transport: domain.TransportHTTP,
		URL: "https://mcp.example.com/" + name, Enabled: true}
}

func TestWriteMCPServer_anAreasRemoteServer_isStoredAtTheArea(t *testing.T) {
	i := newIntegrations(t)
	allowEgress(t, i, domain.Scope{Company: "acme"}, "https://mcp.example.com")
	ctx := context.Background()
	if _, err := i.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: "usr_cx", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope,
		Server: remoteServer("cx-jira"), Given: domain.MCPCredentialPatch{Token: ptr("cx-token")},
	}); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}
	read, err := i.MCPServerNamed(ctx, "cx-jira")
	if err != nil {
		t.Fatalf("MCPServerNamed: %v", err)
	}
	creds, err := i.MCPCredentials(ctx, "cx-jira")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if read.Scope != cxScope || creds.Token != "cx-token" {
		t.Errorf("read %s with token %q, want the area's", read.Scope, creds.Token)
	}
}

func TestWriteMCPServer_anAreasLocalServer_isRefused(t *testing.T) {
	i := newIntegrations(t)
	_, err := i.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_cx", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope,
		Server: domain.MCPServer{Name: "cx-local", Transport: domain.TransportStdio,
			Command: "sh", AcceptsLocalExecution: true, Enabled: true},
	})
	if !errors.Is(err, admin.ErrAreaServerIsRemote) {
		t.Fatalf("err = %v, want ErrAreaServerIsRemote", err)
	}
}

// One name, one server: an area cannot take a name the platform holds.
func TestWriteMCPServer_aNameThePlatformHolds_isTaken(t *testing.T) {
	i := newIntegrations(t)
	allowEgress(t, i, domain.Scope{Company: "acme"}, "https://mcp.example.com")
	ctx := context.Background()
	if err := i.PutMCPServer(ctx, "usr_ana", platform, remoteServer("jira"), domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("the platform's server: %v", err)
	}
	_, err := i.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: "usr_cx", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope, Server: remoteServer("jira"),
	})
	if !errors.Is(err, settings.ErrNameTaken) {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
}

/*
A removal and a creation of one name take turns (NT-019).

A removal that finds the name nowhere clears what is left under it: personal
credentials, observations. A creation in another scope landing between that
decision and the clearing would lose its new credential. Both hold one lock per
name for their whole transaction, so the creation waits for the removal.
*/
func TestMCPServerWrites_ofOneName_waitForTheLockOnTheName(t *testing.T) {
	i := newIntegrations(t)
	allowEgress(t, i, domain.Scope{Company: "acme"}, "https://mcp.example.com")
	pool := openPool(t)
	ctx := context.Background()

	for _, door := range []struct {
		name string
		act  func() error
	}{
		{"write", func() error {
			_, err := i.WriteMCPServer(ctx, admin.MCPServerWrite{
				By: "usr_fin", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope, Server: remoteServer("jira"),
			})
			return err
		}},
		{"remove", func() error {
			return i.RemoveMCPServer(ctx, admin.Removal{By: "usr_ana", Scope: platform, Name: "jira"})
		}},
	} {
		holder, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := admin.LockMCPServerName(ctx, holder, "jira"); err != nil {
			t.Fatalf("hold the name: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- door.act() }()
		waited := waitForAdvisoryWait(t, pool)
		select {
		case err := <-done:
			_ = holder.Rollback(ctx)
			t.Fatalf("%s finished (%v) while the name was held", door.name, err)
		default:
		}
		_ = holder.Rollback(ctx)
		if err := <-done; err != nil {
			t.Fatalf("%s after the name was released: %v", door.name, err)
		}
		if !waited {
			t.Errorf("%s never waited on the name's lock", door.name)
		}
	}
}

// waitForAdvisoryWait reports whether a backend came to wait on an advisory
// lock within a few seconds.
func waitForAdvisoryWait(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity
			where datname = current_database() and wait_event_type = 'Lock' and wait_event = 'advisory'`,
		).Scan(&waiting); err != nil {
			t.Fatalf("read pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
