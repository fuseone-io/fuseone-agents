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
the claim alone, so neither another platform tool nor a near miss of the id
borrows it. The effect must be a write. And the engine executes these ids
itself against the ledger and never hands them to a tool server, so a server
registering a colliding name gains nothing. Everything else still applies: the
pack (only agents that declare it), the data barrier, the budget, an authored
policy that names it, and the draft stage, which never acts unsupervised.
*/
func coordinationWrite(r Request) bool {
	return r.Tool.Coordination() && r.Effect == domain.EffectWrite
}

// coordinationMisclassified is a coordination tool arriving as anything but a
// write — a wiring mistake that would make it cacheable or ladder-bound.
func coordinationMisclassified(r Request) bool {
	return r.Tool.Coordination() && r.Effect != domain.EffectWrite
}
