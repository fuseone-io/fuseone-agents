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
func TestSweep_runsNobodyCanBeReached_doNotStarveOneThatCanBe(t *testing.T) {
	store, pool := channelStore(t)

	// Oldest, and answerable: somebody may decide in cx and has an account.
	answerable := domain.Scope{Company: "acme", Area: "cx"}
	awaitApprovalIn(t, pool, answerable, "run-answerable")

	// Newer, and unanswerable: nobody who may decide in ops has linked an
	// account. A page of two means these fill it.
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

	if _, err := r.Sweep(context.Background(), 2); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %+v, want nothing on the first page", posts.sent)
	}

	if _, err := r.Sweep(context.Background(), 2); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if !addressed(posts.sent, "U-ana") {
		t.Fatalf("sent %+v, want the run that could be answered", posts.sent)
	}
}

/*
And the same for the exit that is by far the most common.

An agent that asked for nothing, stopping in an area nobody mapped to a
conversation, has no destination at all — so nothing in the fan-out has anything
to say about it, and it was the one silence with no record against it. A handful
of those sat at the front of every page, and an area that *was* configured never
got its turn.
*/
func TestSweep_runsWithNoDestination_doNotStarveOneWithARoom(t *testing.T) {
	store, pool := channelStore(t)

	configured := domain.Scope{Company: "acme", Area: "cx"}
	awaitApprovalIn(t, pool, configured, "run-in-a-room")

	unmapped := domain.Scope{Company: "acme", Area: "ops"}
	awaitApprovalIn(t, pool, unmapped, "run-nowhere-1")
	awaitApprovalIn(t, pool, unmapped, "run-nowhere-2")

	posts := &recorder{}
	// No agent asks for anything: the default policy, which is what every
	// agent published before this existed has.
	r := channel.NewReporter(store,
		roomsByScope{configured: room("C07-cx", false)}, posts,
		func() time.Time { return time.Now() }, nil).
		WithDeliveries(store).
		WithOwnerApprovals(wanting(domain.ApprovalPolicy{}), oneConnection)

	if _, err := r.Sweep(context.Background(), 2); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %+v, want nothing on the first page", posts.sent)
	}

	if _, err := r.Sweep(context.Background(), 2); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if !addressed(posts.sent, "C07-cx") {
		t.Fatalf("sent %+v, want the run whose area has a conversation", posts.sent)
	}
}

// roomsByScope is which scopes have a conversation and which have none, which
// is the ordinary shape of an installation part-way through being configured.
type roomsByScope map[domain.Scope]channel.Conversation

func (r roomsByScope) For(
	_ context.Context, scope domain.Scope,
) ([]channel.Conversation, error) {
	if place, mapped := r[scope]; mapped {
		return []channel.Conversation{place}, nil
	}
	return nil, nil
}

// decidersByScope is who may decide, by scope: one area answerable, another not.
type decidersByScope map[domain.Scope][]domain.UserID

func (d decidersByScope) ApproversIn(
	_ context.Context, scope domain.Scope,
) ([]domain.UserID, error) {
	return d[scope], nil
}
