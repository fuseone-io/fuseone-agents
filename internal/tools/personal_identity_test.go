package tools_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

/*
A session is opened as the identity the pool read, and every request on it is
checked against that one (fourth review of #483). A request refused because
the credential has moved on never reached the server, so the pool retires the
session and runs the call again on one opened as the new identity — a bounded
number of times, so a credential that never holds still is an error, not a
loop.
*/

func TestInvoke_theOpen_isAskedForTheIdentityThePoolRead(t *testing.T) {
	t.Parallel()
	o, ids := &opener{}, &identities{ids: map[domain.UserID]string{"usr_ana": "first"}}
	c := personalWithIdentity(t, o, ids)

	if _, err := callAs(t.Context(), c, "usr_ana"); err != nil {
		t.Fatalf("call: %v", err)
	}
	if !slices.Equal(o.as, []string{"first"}) {
		t.Errorf("opened as %v, want [first]", o.as)
	}
}

// identityMoved is the refusal the credential layer gives a request whose
// credential no longer speaks as its session, wrapped as the SDK wraps it.
var identityMoved = fmt.Errorf("calling \"tools/call\": rejected: %w", tools.ErrSessionIdentityChanged)

func TestInvoke_aCallRefusedForAMovedIdentity_retiresTheSessionAndRunsOnANewOne(t *testing.T) {
	t.Parallel()
	o := &opener{}
	o.shape = func(s *personSession) {
		if len(o.opened) == 0 {
			s.err = identityMoved
		}
	}
	ids := &identities{ids: map[domain.UserID]string{"usr_ana": "first"}}
	c := personalWithIdentity(t, o, ids)

	res, err := callAs(t.Context(), c, "usr_ana")
	if err != nil || res.Failed {
		t.Fatalf("call = %+v, %v; want it run on a new session", res, err)
	}
	if n := len(o.whoOpened()); n != 2 || !o.session(0).isClosed() || o.session(1).calls != 1 {
		t.Errorf("opened %d, first closed=%v; want the refused session retired and the call on a new one",
			n, o.session(0).isClosed())
	}
}

func TestInvoke_anOpenRefusedForAMovedIdentity_isTriedAgain(t *testing.T) {
	t.Parallel()
	o := &opener{fail: identityMoved}
	c := personalWithIdentity(t, o, &identities{ids: map[domain.UserID]string{"usr_ana": "first"}})

	if res, err := callAs(t.Context(), c, "usr_ana"); err != nil || res.Failed {
		t.Fatalf("call = %+v, %v; want it run once the open held", res, err)
	}
	if len(o.as) != 2 {
		t.Errorf("opens asked %d, want the refused one and the one that held", len(o.as))
	}
}

// Every session the opener opens has its calls refused as moved.
func TestInvoke_aCredentialThatNeverHoldsStill_failsTheCallAfterBoundedTries(t *testing.T) {
	t.Parallel()
	o := &opener{}
	o.shape = func(s *personSession) { s.err = identityMoved }
	c := personalWithIdentity(t, o, &identities{ids: map[domain.UserID]string{"usr_ana": "first"}})

	res, err := callAs(context.Background(), c, "usr_ana")
	if err == nil && !res.Failed {
		t.Fatalf("call = %+v; want it failed", res)
	}
	if n := len(o.whoOpened()); n != 3 {
		t.Errorf("opened %d, want three tries and no more", n)
	}
}
