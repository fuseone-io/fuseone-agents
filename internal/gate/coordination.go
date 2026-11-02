package gate

import "github.com/fuseone/agents/internal/domain"

/*
coordinationWrite is an internal coordination operation, stated rather than
inferred (#206, NT-013).

Platform-owned settles a tool's envelope and its availability, never its
effect: a ticket answer is platform-owned and reaches a person. The work-item
claim is different in kind. It changes only the platform's own record of
which run works on which card, and nothing leaves the installation. Held to
the write ladder it would park for a person before reserving the card — and
the race the claim exists to close, two runs of one role picking the same card
while the first waits on an approval, would reopen exactly there. So:

  - it passes the taint, autonomy (copilot) and built-in policy checks without
    approval, even after an untrusted read, because untrusted content steering
    the claim can at worst claim the wrong card, which the run already could;
  - it must be recorded as a write. Its answer depends on every other open run
    at the moment it is asked, so it is never a read and never served from a
    cache — checkContract refuses any other classification.

The exemption is closed three ways. ToolID.Coordination is an explicit list,
the claim and the link, so neither another platform tool nor a near miss of
the id borrows it. The effect must be a write. And the engine executes these ids
itself against the ledger and never hands them to a tool server, so a server
registering a colliding name gains nothing. Everything else still applies: the
pack (only agents that declare it), the data barrier, the budget, an authored
policy that names it, and the draft stage, which never acts unsupervised.
*/
//
// The link joins it under one more condition (#204, slice 2b): the engine has
// proven it from the run's own record — the card the run holds is the root,
// and the run's create_pull_request answered naming the pull request. Untrusted
// content steering a proven link can at worst link the run's own pull request
// to the run's own card. An unproven link is an ordinary write and keeps every
// rule a write lives under; the approval it parks for is its proof.
func coordinationWrite(r Request) bool {
	if r.Tool == domain.ToolWorkItemLink && !r.LinkProven {
		return false
	}
	return r.Tool.Coordination() && r.Effect == domain.EffectWrite
}

// coordinationMisclassified is a coordination tool arriving as anything but a
// write — a wiring mistake that would make it cacheable or ladder-bound.
func coordinationMisclassified(r Request) bool {
	return r.Tool.Coordination() && r.Effect != domain.EffectWrite
}

// linkUnmakeable is a link the engine found the run's own record refuses
// before anything is asked: no card held, or a reference that is not a pull
// request of the card's repository. Refused as a contract the model can fix,
// never parked for a person to approve what the ledger would refuse.
func linkUnmakeable(r Request) (string, bool) {
	return r.LinkRefusal, r.Tool == domain.ToolWorkItemLink && r.LinkRefusal != ""
}
