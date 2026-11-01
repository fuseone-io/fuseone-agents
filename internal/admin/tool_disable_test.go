package admin_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

func saveCRM(t *testing.T, i *admin.Integrations, enabled bool) {
	t.Helper()
	if err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{
		Name: "crm", Command: "/usr/local/bin/crm-mcp", Enabled: enabled, AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer enabled=%v: %v", enabled, err)
	}
}

// generationOf is what a worker connecting now would read for the server.
func generationOf(t *testing.T, i *admin.Integrations, name string) time.Time {
	t.Helper()
	servers, err := i.MCPServers(context.Background())
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	for _, s := range servers {
		if s.Name == name {
			return s.UpdatedAt
		}
	}
	t.Fatalf("%s is not configured", name)
	return time.Time{}
}

/*
Switching a server off takes its tools off the console in the same act.

Waiting for a worker to notice is not enough: the worker that published them
may be gone, and one that never reached the server has nothing to withdraw,
so nothing would ever publish — and the publish check would go on accepting
a role that reaches a server no run can call.
*/
func TestPutMCPServer_switchingAServerOff_removesItsToolsAndSaysHowMany(t *testing.T) {
	i := newIntegrations(t)
	curator := freshCurator(t)
	saveCRM(t, i, true)
	publish(t, curator, discovered("crm", generationOf(t, i, "crm"), "crm.lookup", "crm.note"))

	saveCRM(t, i, false)

	if got := offered(t, curator, "crm"); len(got) != 0 {
		t.Fatalf("a switched-off server still offers %v", got)
	}
	var detail map[string]any
	var raw []byte
	if err := openPool(t).QueryRow(context.Background(),
		`select detail from admin_events where target = 'crm' and action = 'mcp_server.configured'
		 order by event_id desc limit 1`).Scan(&raw); err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("decode the trail: %v", err)
	}
	if detail["toolsRemoved"] != float64(2) {
		t.Fatalf("trail detail %v, want toolsRemoved 2", detail)
	}
}

// Saving a server that stays on is not a removal: its tools stay until a
// discovery says otherwise.
func TestPutMCPServer_savingAServerThatStaysOn_keepsItsTools(t *testing.T) {
	i := newIntegrations(t)
	curator := freshCurator(t)
	saveCRM(t, i, true)
	publish(t, curator, discovered("crm", generationOf(t, i, "crm"), "crm.lookup"))

	saveCRM(t, i, true)

	if got := offered(t, curator, "crm"); len(got) != 1 {
		t.Fatalf("crm offers %v after a save that kept it on, want its tool kept", got)
	}
}

/*
Two people at once: one switches the server off, the other removes it. Both
take the catalogue's lock before the server's row, so one simply waits for
the other. Taken the other way round by either, each would hold what the
other needs, and Postgres would abort one of them as a deadlock.
*/
func TestPutMCPServer_switchedOffWhileSomebodyRemovesIt_neitherIsAbortedAsADeadlock(t *testing.T) {
	i := newIntegrations(t)
	for round := range 20 {
		saveCRM(t, i, true)
		done := make(chan error, 2)
		go func() {
			done <- i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{
				Name: "crm", Command: "/usr/local/bin/crm-mcp", AcceptsLocalExecution: true,
			}, domain.MCPCredentialPatch{})
		}()
		go func() { done <- i.DeleteMCPServer(context.Background(), "usr_ana", platform, "crm") }()
		for range 2 {
			if err := <-done; err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
	}
}

/*
A worker publishing while somebody switches the server off. Both take the
catalogue's lock before anything else they touch, in the same order the
removal uses — the lock, then the server's row — so neither waits on the
other holding what it needs. Whichever goes first, the server ends with no
tools: a publication recorded after the switch is judged against it, one
recorded before is undone by it.
*/
func TestPutMCPServer_switchedOffWhileAWorkerPublishes_endsWithNoTools(t *testing.T) {
	i := newIntegrations(t)
	curator := freshCurator(t)
	for round := range 20 {
		saveCRM(t, i, true)
		pub := discovered("crm", generationOf(t, i, "crm"), "crm.lookup")
		publish(t, curator, pub)

		done := make(chan error, 2)
		go func() { done <- curator.Publish(context.Background(), pub) }()
		go func() {
			done <- i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{
				Name: "crm", Command: "/usr/local/bin/crm-mcp", AcceptsLocalExecution: true,
			}, domain.MCPCredentialPatch{})
		}()
		for range 2 {
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("round %d: %v", round, err)
				}
			case <-time.After(20 * time.Second):
				t.Fatalf("round %d: a publish and a switch-off are waiting on each other", round)
			}
		}
		if got := offered(t, curator, "crm"); len(got) != 0 {
			t.Fatalf("round %d: a switched-off server offers %v", round, got)
		}
	}
}
