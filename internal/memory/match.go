package memory

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
MatchInput is one identity somebody is about to teach, asked about before they
teach it.

The same fields the canonical key is built from, because the question is exactly
"is this already here" and any looser reading would answer a different one.
*/
type MatchInput struct {
	Scope     domain.Scope
	AgentID   domain.AgentID
	Kind      string
	Subject   string
	Signature string
	Now       time.Time
}

/*
Match is what the platform already holds for an identity, in every state it
could be in.

Every state, deliberately. An expired memory is the one that catches people out:
Find does not return it, so the fact appears to be unknown, and somebody teaches
it again and cannot see why the duplicate exists. Disabled says somebody decided
against it, which is worth reading before deciding for it again. And
source_erased says the platform lost the proof — reteaching it will not bring
that back.

Shared is separate from the agent's own because the two mean different things to
the person asking. Their own memory is theirs to correct; shared memory is what
every agent in the scope reads, and improving it is an act taken against the
shared row rather than from here.
*/
type Match struct {
	// Own is the memory in the namespace being taught, if there is one.
	Own *domain.MemoryAssertion
	// Shared is the memory every agent in the scope reads, when an
	// agent-scoped question is covered by one. Never set for a shared
	// question: nothing covers shared memory but itself.
	Shared *domain.MemoryAssertion
	// Pending is the proposal nobody has decided yet, if the queue holds one
	// for this identity.
	Pending *domain.MemorySuggestion
}

// Answered is true when there is anything to show. A caller with nothing to
// show should say the fact is new rather than render an empty panel.
func (m Match) Answered() bool {
	return m.Own != nil || m.Shared != nil || m.Pending != nil
}

/*
identityOf is the assertion shape a match question asks about.

Claim and evidence are absent because neither is part of the identity: two
people teaching the same fact in different words are teaching the same fact, and
that is the whole reason the canonical key exists.
*/
func (in MatchInput) identityOf(agent domain.AgentID) domain.MemoryAssertion {
	a := domain.MemoryAssertion{
		Scope: in.Scope, AgentID: agent, Kind: clean(in.Kind),
		Subject: clean(in.Subject), Signature: clean(in.Signature),
	}
	a.ID = domain.MemoryAssertionID(a)
	return a
}

/*
Match answers what is already here, and never writes.

Deliberately not on the Gate's path. A run recalls memory through Find, which is
bounded and indexed for that; this exists so a person composing one can be shown
what they are about to duplicate, and putting it anywhere a run reaches would
add a database read to every decision the Gate makes.
*/
func (m *Memory) Match(ctx context.Context, in MatchInput) (Match, error) {
	if err := ctx.Err(); err != nil {
		return Match{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out Match
	own, err := m.byIdentity(in.identityOf(in.AgentID))
	if err != nil {
		return Match{}, err
	}
	out.Own = own
	if in.AgentID != "" {
		shared, err := m.byIdentity(in.identityOf(""))
		if err != nil {
			return Match{}, err
		}
		out.Shared = shared
	}
	out.Pending = m.pendingFor(in)
	return out, nil
}

func (m *Memory) pendingFor(in MatchInput) *domain.MemorySuggestion {
	want := in.identityOf(in.AgentID).ID
	for _, held := range m.suggestions {
		if held.Status != domain.MemorySuggestionPending || held.AssertionID != want {
			continue
		}
		found := cloneSuggestion(held)
		return &found
	}
	return nil
}
