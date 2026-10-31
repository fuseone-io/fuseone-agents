package domain

import "fmt"

/*
StandingPolicy is whether a standing approval may ever release a call of this
agent.

Declared on the agent definition, so it is reviewed like every other line of
it and frozen with the version a run pins. Absent is today's behaviour: a
grant somebody created may cover a parked call. Forbidden means every approval
the Gate asks of this agent is answered by a person, one at a time — which is
what a pilot measuring how often each role is right needs, since a grant that
answered for them would count as a person's approval that nobody gave (#192).

Only the forbidding value is written down. "allowed" is the absence of a line,
not a second spelling of it: two spellings of one meaning would give one
definition two digests, and republishing it from the console would mint a
version of identical intent.
*/
type StandingPolicy string

const (
	// StandingAllowed is the zero value: no line in the file.
	StandingAllowed StandingPolicy = ""
	// StandingForbidden refuses every standing grant, at creation and at claim.
	StandingForbidden StandingPolicy = "forbidden"
)

// Forbidden reports whether no standing grant may cover this agent's calls.
func (p StandingPolicy) Forbidden() bool { return p == StandingForbidden }

// Validate refuses a value nobody can act on. Read as allowed, a typo in the
// one line that was meant to forbid grants would quietly permit them.
func (p StandingPolicy) Validate() error {
	switch p {
	case StandingAllowed, StandingForbidden:
		return nil
	}
	return fmt.Errorf("standing_approvals must be forbidden or absent, not %q", string(p))
}
