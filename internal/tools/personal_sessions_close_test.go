package tools_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A session being closed is still open: closing one is a request to its server,
and until it returns the connection is there. Its place against the limit comes
free only then, or slow closes, one after another, would hold more connections
than the limit allows (third review of #483).
*/

func TestInvoke_aRetiredSessionStillClosing_keepsCountingAgainstTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		hold, closing := make(chan struct{}), make(chan struct{})
		o := &opener{}
		o.shape = func(s *personSession) {
			if len(o.opened) == 0 {
				s.hold, s.closeHold = hold, closing
			}
		}
		ids := &identities{ids: map[domain.UserID]string{"usr_ana": "first"}}
		c := tools.NewCatalog(engine.NewMemoryContent())
		if err := c.AddServer(context.Background(), "crm", lookupServer(), nil, tools.WithPersonalSessions(tools.PersonalSessions{
			Open: o.open, Identity: ids.of, Limit: 1,
		})); err != nil {
			t.Fatalf("AddServer: %v", err)
		}
		first := make(chan error, 1)
		go func() { _, err := callAs(t.Context(), c, "usr_ana"); first <- err }()
		synctest.Wait()

		ids.set("usr_ana", "second")
		second := make(chan error, 1)
		go func() { _, err := callAs(t.Context(), c, "usr_ana"); second <- err }()
		synctest.Wait()
		close(hold) // the first call returns, and its retired session starts closing
		synctest.Wait()
		if n := len(o.whoOpened()); n != 1 {
			t.Errorf("opened %d while the retired session was still closing, want the call waiting at the limit of 1", n)
		}
		close(closing)
		if err := <-first; err != nil {
			t.Fatalf("first call: %v", err)
		}
		if err := <-second; err != nil {
			t.Fatalf("second call: %v", err)
		}
		if n := len(o.whoOpened()); n != 2 {
			t.Errorf("opened %d, want the new one once the old one had closed", n)
		}
	})
}

// The same for an idle session another person's call closes as it expires.
func TestInvoke_anExpiredSessionStillClosing_keepsCountingAgainstTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		closing := make(chan struct{})
		o := &opener{}
		o.shape = func(s *personSession) {
			if len(o.opened) == 0 {
				s.closeHold = closing
			}
		}
		c, clk := personal(t, o, 2, lookupServer())
		if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
			t.Fatalf("Ana's call: %v", err)
		}
		clk.advance(6 * time.Minute)
		if _, err := callAs(t.Context(), c, "usr_bia"); err != nil {
			t.Fatalf("Bia's call: %v", err)
		}
		clk.advance(5 * time.Minute) // Ana's session is past its idle time, Bia's is not

		bia := make(chan error, 1)
		go func() { _, err := callAs(t.Context(), c, "usr_bia"); bia <- err }() // closes Ana's
		synctest.Wait()
		caio := make(chan error, 1)
		go func() { _, err := callAs(t.Context(), c, "usr_caio"); caio <- err }()
		synctest.Wait()
		if n := len(o.whoOpened()); n != 2 {
			t.Errorf("opened %d while Ana's session was still closing, want Caio waiting at the limit of 2", n)
		}
		close(closing)
		if err := <-bia; err != nil {
			t.Fatalf("Bia's second call: %v", err)
		}
		if err := <-caio; err != nil {
			t.Fatalf("Caio's call: %v", err)
		}
	})
}
