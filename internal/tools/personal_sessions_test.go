package tools_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A server that takes each person's own credential gets a session per person:
opened with that person's credential from its initialize, because a server
that knows who is calling by the token learns it there.

Three things hold whatever the load (reviewed 2026-11-25):
  - a session with a call in flight is never closed to make room; at the cap,
    a call waits for a place, and gives up when its context does;
  - a session is offered to calls only once it is open; one that fails to
    open leaves nothing behind;
  - only a dead connection retires a session, never a tool's own error.
*/

// personSession is one person's connection. hold, when set, keeps every call
// inside CallTool until it is closed.
type personSession struct {
	mu     sync.Mutex
	who    domain.UserID
	calls  int
	closed bool
	hold   chan struct{}
	result *mcp.CallToolResult
	err    error
	// callHook, when set, decides each call: what to hold it on, and its error.
	callHook func() (chan struct{}, error)
	// closeHold, when set, keeps Close from returning until it is closed.
	closeHold chan struct{}
}

func (s *personSession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{}, nil
}

func (s *personSession) CallTool(ctx context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.mu.Lock()
	s.calls++
	hold, result, err := s.hold, s.result, s.err
	hook := s.callHook
	s.mu.Unlock()
	if hook != nil {
		hold, err = hook()
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return text("ok"), nil
	}
	return result, nil
}

func (s *personSession) Close() error {
	s.mu.Lock()
	hold := s.closeHold
	s.mu.Unlock()
	if hold != nil {
		<-hold
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *personSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// opener opens a personSession per call and remembers each. fail, when set,
// refuses the next open; shape, when set, prepares each session it opens.
type opener struct {
	mu     sync.Mutex
	opened []*personSession
	fail   error
	gate   chan struct{}
	shape  func(*personSession)
}

func (o *opener) open(ctx context.Context, who domain.UserID) (tools.Session, error) {
	o.mu.Lock()
	gate, fail := o.gate, o.fail
	o.fail = nil
	o.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail != nil {
		return nil, fail
	}
	s := &personSession{who: who}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.shape != nil {
		o.shape(s)
	}
	o.opened = append(o.opened, s)
	return s, nil
}

func (o *opener) whoOpened() []domain.UserID {
	o.mu.Lock()
	defer o.mu.Unlock()
	who := make([]domain.UserID, len(o.opened))
	for i, s := range o.opened {
		who[i] = s.who
	}
	return who
}

func (o *opener) session(i int) *personSession {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.opened[i]
}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// personal is a catalogue with crm served per person, at most limit at once.
func personal(t *testing.T, o *opener, limit int, shared *fakeServer) (*tools.Catalog, *clock) {
	t.Helper()
	clk := &clock{now: time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)}
	c := tools.NewCatalog(engine.NewMemoryContent()).WithClock(clk.Now)
	if err := c.AddServer(context.Background(), "crm", shared, nil, tools.WithPersonalSessions(tools.PersonalSessions{
		Open: o.open, Limit: limit, Idle: 10 * time.Minute,
	})); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	return c, clk
}

func callAs(ctx context.Context, c *tools.Catalog, who domain.UserID) (engine.ToolResult, error) {
	return c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.lookup", Args: []byte(`{}`), OnBehalfOf: who})
}

func TestInvoke_aPersonalServer_givesEachPersonTheirOwnSession(t *testing.T) {
	t.Parallel()
	o, shared := &opener{}, lookupServer()
	c, _ := personal(t, o, 8, shared)

	for _, who := range []domain.UserID{"usr_ana", "usr_bia", "usr_ana"} {
		if _, err := callAs(t.Context(), c, who); err != nil {
			t.Fatalf("call as %s: %v", who, err)
		}
	}
	if got := o.whoOpened(); !slices.Equal(got, []domain.UserID{"usr_ana", "usr_bia"}) {
		t.Fatalf("opened for %v, want one session for Ana and one for Bia", got)
	}
	if o.session(0).calls != 2 || len(shared.calls) != 0 {
		t.Errorf("Ana's session took %d calls and the shared one %d, want 2 and none",
			o.session(0).calls, len(shared.calls))
	}
}

// With no person the call is the shared session's, whose credential policy
// refuses a call that needs someone.
func TestInvoke_aPersonalServer_aCallWithNoPersonStaysOnTheSharedSession(t *testing.T) {
	t.Parallel()
	o, shared := &opener{}, lookupServer()
	c, _ := personal(t, o, 8, shared)

	if _, err := callAs(t.Context(), c, ""); err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(o.whoOpened()) != 0 || len(shared.calls) != 1 {
		t.Errorf("opened %v and the shared session took %d calls, want nothing opened and one shared call",
			o.whoOpened(), len(shared.calls))
	}
}

func TestInvoke_aPersonsFirstCallsTogether_openOneSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := &opener{gate: make(chan struct{})}
		c, _ := personal(t, o, 8, lookupServer())
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { _, _ = callAs(t.Context(), c, "usr_ana") })
		}
		synctest.Wait()
		close(o.gate)
		wg.Wait()
		if got := o.whoOpened(); len(got) != 1 {
			t.Errorf("opened %v, want one session for Ana's two first calls", got)
		}
	})
}

func TestInvoke_atTheLimitWithEverySessionBusy_waitsForAPlace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		o := &opener{shape: func(s *personSession) {
			if s.who == "usr_ana" {
				s.hold = hold
			}
		}}
		c, _ := personal(t, o, 1, lookupServer())
		go func() { _, _ = callAs(t.Context(), c, "usr_ana") }()
		synctest.Wait()

		done := make(chan error, 1)
		go func() { _, err := callAs(t.Context(), c, "usr_bia"); done <- err }()
		synctest.Wait()
		if got := o.whoOpened(); len(got) != 1 || o.session(0).isClosed() {
			t.Fatalf("opened %v, Ana's closed=%v; want Bia waiting and Ana's busy session open",
				got, o.session(0).isClosed())
		}

		close(hold)
		if err := <-done; err != nil {
			t.Fatalf("Bia's call: %v", err)
		}
		if got := o.whoOpened(); !slices.Equal(got, []domain.UserID{"usr_ana", "usr_bia"}) || !o.session(0).isClosed() {
			t.Errorf("opened %v, Ana's closed=%v; want Bia's opened once Ana's was idle and closed",
				got, o.session(0).isClosed())
		}
	})
}

func TestInvoke_atTheLimitWithEverySessionBusy_aCallThatGivesUpOpensNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold := make(chan struct{})
		defer close(hold)
		o := &opener{shape: func(s *personSession) { s.hold = hold }}
		c, _ := personal(t, o, 1, lookupServer())
		go func() { _, _ = callAs(t.Context(), c, "usr_ana") }()
		synctest.Wait()

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { _, err := callAs(ctx, c, "usr_bia"); done <- err }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Bia's call = %v, want it to give up with its context", err)
		}
		if got := o.whoOpened(); len(got) != 1 || o.session(0).isClosed() {
			t.Errorf("opened %v, Ana's closed=%v; want nothing opened for Bia and Ana's untouched",
				got, o.session(0).isClosed())
		}
	})
}

func TestInvoke_atTheLimit_closesTheLeastRecentlyUsedIdleSession(t *testing.T) {
	t.Parallel()
	o := &opener{}
	c, clk := personal(t, o, 2, lookupServer())
	for _, who := range []domain.UserID{"usr_ana", "usr_bia", "usr_ana", "usr_cai"} {
		clk.advance(time.Second)
		if _, err := callAs(t.Context(), c, who); err != nil {
			t.Fatalf("call as %s: %v", who, err)
		}
	}
	ana, bia := o.session(0), o.session(1)
	if ana.isClosed() || !bia.isClosed() {
		t.Errorf("Ana's closed=%v, Bia's closed=%v; want Bia's, the least recently used, closed for Cai",
			ana.isClosed(), bia.isClosed())
	}
}

func TestInvoke_aSessionThatFailsToOpen_leavesNothingBehind(t *testing.T) {
	t.Parallel()
	o := &opener{fail: errors.New("initialize: Unauthorized")}
	c, _ := personal(t, o, 8, lookupServer())

	if res, err := callAs(t.Context(), c, "usr_ana"); err == nil || !res.Failed {
		t.Fatalf("first call = %+v, %v; want it failed with the open", res, err)
	}
	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := o.whoOpened(); len(got) != 1 || o.session(0).calls != 1 {
		t.Errorf("opened %v, want one session, opened afresh by the second call", got)
	}
}

func TestInvoke_aToolsOwnError_keepsThePersonsSession(t *testing.T) {
	t.Parallel()
	o := &opener{shape: func(s *personSession) {
		s.result = &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "no such customer"}}}
	}}
	c, _ := personal(t, o, 8, lookupServer())
	for range 2 {
		if res, _ := callAs(t.Context(), c, "usr_ana"); !res.Failed {
			t.Fatalf("result = %+v, want the tool's error", res)
		}
	}
	if got := o.whoOpened(); len(got) != 1 || o.session(0).isClosed() {
		t.Errorf("opened %v, closed=%v; want the one session kept", got, o.session(0).isClosed())
	}
}

// A failure that leaves the connection usable is not a reason to reopen it.
func TestInvoke_aFailureThatLeavesTheConnectionUsable_keepsThePersonsSession(t *testing.T) {
	t.Parallel()
	o := &opener{shape: func(s *personSession) { s.err = errors.New("Bad Request: Unsupported protocol version") }}
	c, _ := personal(t, o, 8, lookupServer())
	for range 2 {
		if _, err := callAs(t.Context(), c, "usr_ana"); err == nil {
			t.Fatal("call succeeded, want the failure")
		}
	}
	if got := o.whoOpened(); len(got) != 1 || o.session(0).isClosed() {
		t.Errorf("opened %v, closed=%v; want the one session kept", got, o.session(0).isClosed())
	}
}

func TestInvoke_aDeadConnection_retiresThePersonsSession(t *testing.T) {
	t.Parallel()
	// The first session's connection is dead; shape runs under the opener's
	// lock, so it reads what was opened directly.
	o := &opener{}
	o.shape = func(s *personSession) {
		if len(o.opened) == 0 {
			s.err = closedErr
		}
	}
	c, _ := personal(t, o, 8, lookupServer())

	_, _ = callAs(t.Context(), c, "usr_ana")
	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("call after the connection died: %v", err)
	}
	if got := o.whoOpened(); len(got) != 2 || !o.session(0).isClosed() {
		t.Errorf("opened %v, first closed=%v; want the dead one closed and a new one opened",
			got, o.session(0).isClosed())
	}
}

func TestInvoke_anIdleSession_isClosedByTheNextCall(t *testing.T) {
	t.Parallel()
	o := &opener{}
	c, clk := personal(t, o, 8, lookupServer())
	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("Ana's call: %v", err)
	}
	clk.advance(11 * time.Minute)
	if _, err := callAs(t.Context(), c, "usr_bia"); err != nil {
		t.Fatalf("Bia's call: %v", err)
	}
	if !o.session(0).isClosed() {
		t.Error("Ana's session, idle past its limit, is still open")
	}
}

func TestRemoveServer_closesEveryPersonsSession(t *testing.T) {
	t.Parallel()
	o := &opener{}
	c, _ := personal(t, o, 8, lookupServer())
	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("Ana's call: %v", err)
	}
	if err := c.RemoveServer("crm"); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	if !o.session(0).isClosed() {
		t.Error("Ana's session outlived its server")
	}
}
