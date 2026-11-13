package admin_test

import (
	"context"
	"errors"
	"testing"

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
