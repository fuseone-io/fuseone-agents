package tools_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
What review of #483 found the pool did not hold (2026-11-26):
  - a person's session is opened under one identity of their credential, and
    a deliberate change of it must not ride a session opened under the old;
  - a call that read the server before a reconnect runs on what it read, not
    on the sessions of the definition that replaced it;
  - closing a server's sessions is I/O, and none of it happens while the
    catalogue is locked against every other server's calls.
*/

// identities is who each person's credential currently speaks as.
type identities struct {
	mu  sync.Mutex
	ids map[domain.UserID]string
	err error
}

func (i *identities) of(_ context.Context, who domain.UserID) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.ids[who], i.err
}

func (i *identities) set(who domain.UserID, id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ids[who] = id
}

func personalWithIdentity(t *testing.T, o *opener, ids *identities) *tools.Catalog {
	t.Helper()
	c := tools.NewCatalog(engine.NewMemoryContent())
	if err := c.AddServer(context.Background(), "crm", lookupServer(), nil, tools.WithPersonalSessions(tools.PersonalSessions{
		Open: o.open, Identity: ids.of, Limit: 8, Idle: 10 * time.Minute,
	})); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	return c
}

func TestInvoke_aCredentialChangedDeliberately_opensANewSession(t *testing.T) {
	t.Parallel()
	o, ids := &opener{}, &identities{ids: map[domain.UserID]string{"usr_ana": "first"}}
	c := personalWithIdentity(t, o, ids)

	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	ids.set("usr_ana", "second")
	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("call after the change: %v", err)
	}
	if n := len(o.whoOpened()); n != 2 || !o.session(0).isClosed() || o.session(1).calls != 1 {
		t.Errorf("opened %d, first closed=%v; want the old session closed and the call on a new one",
			n, o.session(0).isClosed())
	}
}

// A call already in flight on the old identity finishes there: the session is
// retired, not closed under it.
func TestInvoke_aCredentialChangedDuringACall_closesTheOldSessionOnlyWhenItReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		o := &opener{}
		o.shape = func(s *personSession) {
			if len(o.opened) == 0 {
				s.hold = hold
			}
		}
		ids := &identities{ids: map[domain.UserID]string{"usr_ana": "first"}}
		c := personalWithIdentity(t, o, ids)
		done := make(chan error, 1)
		go func() { _, err := callAs(t.Context(), c, "usr_ana"); done <- err }()
		synctest.Wait()

		ids.set("usr_ana", "second")
		if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
			t.Fatalf("call after the change: %v", err)
		}
		if o.session(0).isClosed() {
			t.Fatal("the old session was closed under the call in flight")
		}
		close(hold)
		if err := <-done; err != nil {
			t.Fatalf("the call in flight: %v", err)
		}
		if !o.session(0).isClosed() {
			t.Error("the old session outlived its last call")
		}
	})
}

func TestInvoke_aCredentialThatCannotBeRead_opensNothing(t *testing.T) {
	t.Parallel()
	o, ids := &opener{}, &identities{ids: map[domain.UserID]string{}, err: errors.New("vault: sealed")}
	c := personalWithIdentity(t, o, ids)

	if res, err := callAs(t.Context(), c, "usr_ana"); err == nil || !res.Failed {
		t.Fatalf("call = %+v, %v; want it failed", res, err)
	}
	if got := o.whoOpened(); len(got) != 0 {
		t.Errorf("opened %v, want nothing", got)
	}
}

/*
The call reads its server once. A reconnect that lands after that read must
not hand it the replacement's sessions: the Gate judged the definition the call
read, and the replacement may describe another.
*/
func TestInvoke_aReconnectAfterTheCallReadItsServer_doesNotRunItOnTheReplacement(t *testing.T) {
	t.Parallel()
	before, after := &opener{}, &opener{}
	var c *tools.Catalog
	var once sync.Once
	reconnect := func() time.Time {
		// The catalogue reads its clock just after it lets the lock go,
		// which is the window between reading the server and calling it.
		once.Do(func() {
			if err := c.AddServer(context.Background(), "crm", lookupServer(), nil,
				tools.WithPersonalSessions(tools.PersonalSessions{Open: after.open})); err != nil {
				t.Errorf("reconnect: %v", err)
			}
		})
		return time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	}
	c = tools.NewCatalog(engine.NewMemoryContent())
	if err := c.AddServer(context.Background(), "crm", lookupServer(), nil,
		tools.WithPersonalSessions(tools.PersonalSessions{Open: before.open})); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	c.WithClock(reconnect)

	_, _ = callAs(t.Context(), c, "usr_ana")
	if got := after.whoOpened(); len(got) != 0 {
		t.Errorf("the replacement opened %v for a call that read the server before it", got)
	}
}

// slowClose is a session whose close is a request that takes its time.
type slowClose struct {
	*personSession
	release chan struct{}
}

func (s slowClose) Close() error { <-s.release; return s.personSession.Close() }

func TestRemoveServer_closingItsSessions_doesNotHoldOtherServersCalls(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	defer close(release)
	slow := func(_ context.Context, who domain.UserID) (tools.Session, error) {
		return slowClose{personSession: &personSession{who: who}, release: release}, nil
	}
	c := tools.NewCatalog(engine.NewMemoryContent())
	if err := c.AddServer(context.Background(), "crm", lookupServer(), nil,
		tools.WithPersonalSessions(tools.PersonalSessions{Open: slow})); err != nil {
		t.Fatalf("AddServer crm: %v", err)
	}
	if err := c.AddServer(context.Background(), "erp", erpServer(), nil); err != nil {
		t.Fatalf("AddServer erp: %v", err)
	}
	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("Ana's call: %v", err)
	}
	go func() { _ = c.RemoveServer("crm") }()

	other := make(chan error, 1)
	go func() {
		_, err := c.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 2, Tool: "erp.lookup", Args: []byte(`{}`)})
		other <- err
	}()
	select {
	case err := <-other:
		if err != nil {
			t.Fatalf("erp call: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a call to another server waited for crm's sessions to close")
	}
}

func erpServer() *fakeServer {
	s := lookupServer()
	return s
}
