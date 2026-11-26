package main

import (
	"slices"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A session is opened as one identity of the person's credential, and every
request on it must speak as that one. A change that lands while the session
is opening, or after, must not send a call with the new credential on a
session the server tied to the old: a server that fixes who is calling at
initialize would run it as the old account (fourth review of #483).
*/
func TestConnectServer_aCredentialChangedWhileItsSessionOpens_noCallRunsOnTheOldSession(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	store := &rotatingStore{creds: personalCreds{"usr_ana": {Token: "first"}}}
	var once sync.Once
	ids.beforeInitialize = func(auth string) {
		if auth == "Bearer first" {
			once.Do(func() { store.set("usr_ana", domain.MCPCredentials{Token: "second"}) })
		}
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	server := domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true}
	if err := connectServer(t.Context(), catalog, server, domain.MCPCredentials{Token: "shared"}, nil,
		store, credentialPolicy{requirePersonal: true}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}

	res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: "usr_ana"})
	if err != nil || res.Failed {
		t.Fatalf("call = %+v, %v", res, err)
	}
	want := []string{"Bearer second → Bearer second"}
	if got := ids.callsBy(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want the call only on a session the new credential opened", got)
	}
}
