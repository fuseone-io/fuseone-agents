package channel_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
A run nobody can be told about must not hold the page.

Two rules meet here and nearly cancelled each other. A run nobody could be told
about is kept for a later sweep — otherwise configuring the connection or
binding the account announces nothing, ever. And the sweep takes what has not
been tried before what has, so that one broken destination cannot starve the
rest.

Read together, a run kept with *no attempt recorded against it* is "never tried"
for ever: it sits at the front of every page, and with more of those than fit in
one, an older run that could be told is never reached before it leaves the
window. Silence, produced by the two protections against silence.

So the attempt is written down. It is not a delivery and does not pretend to be
one — it says why nobody was told, which is also what the cockpit needs.
*/
func TestSweep_runsNobodyCanBeToldAbout_doNotStarveOneThatCanBe(t *testing.T) {
	store, pool := channelStore(t)

	// Oldest, and answerable: somebody may decide in cx and has an account.
	answerable := domain.Scope{Company: "acme", Area: "cx"}
	awaitApprovalIn(t, pool, answerable, "run-answerable")

	// Newer, and unanswerable: nobody in ops has linked an account. A page of
	// two means these fill it.
	unanswerable := domain.Scope{Company: "acme", Area: "ops"}
	awaitApprovalIn(t, pool, unanswerable, "run-quiet-1")
	awaitApprovalIn(t, pool, unanswerable, "run-quiet-2")

	posts := &recorder{}
	r := channel.NewReporter(store, rooms(), posts,
		func() time.Time { return time.Now() }, nil).
		WithDeliveries(store).
		WithDirectApprovals(
			decidersByScope{answerable: {"usr_ana"}, unanswerable: {"usr_bob"}},
			accountBook{"acme-slack": {"usr_ana": "U-ana"}}).
		WithOwnerApprovals(wanting(domain.ApprovalPolicy{Direct: true}), oneConnection)

	// The first sweep can only see the two it cannot answer.
	if _, err := r.Sweep(context.Background(), 2); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %+v, want nothing: nobody in ops can be reached", posts.sent)
	}

	// The second must reach the one that was waiting behind them.
	if _, err := r.Sweep(context.Background(), 2); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if !addressed(posts.sent, "U-ana") {
		t.Fatalf("sent %+v, want the run that could be answered", posts.sent)
	}
}

// decidersByScope is who may decide, by scope: one area answerable, another not.
type decidersByScope map[domain.Scope][]domain.UserID

func (d decidersByScope) ApproversIn(
	_ context.Context, scope domain.Scope,
) ([]domain.UserID, error) {
	return d[scope], nil
}
