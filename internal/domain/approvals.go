package domain

import (
	"errors"
	"strings"
)

/*
How an agent's owner asked for human approval to arrive.

The Gate decides *whether* a person must answer; this decides *where they are
asked*. Two different questions, and only the second belongs to the agent's
owner: nobody configures away the need for a decision by choosing how they are
told about it.

It lives in the specification because it is the agent's own property and the
owner is who writes that file. It is versioned with everything else there, so a
run is governed by the preference the version it pinned declared — reading
today's would answer a question about a run with a decision taken after it
started.
*/
type ApprovalPolicy struct {
	// Direct asks for a private message from the channel bot as well as the
	// console. The console is never replaced: a run waiting on a person is
	// answerable there whatever this says.
	Direct bool `json:"direct,omitempty" yaml:"direct,omitempty"`
	/*
		Notify names who should be messaged, among the people who may decide.

		Empty means everyone holding Approver in a scope covering the run,
		which is what a conversation's private cards already do. Naming
		somebody is addressing, never authorising: the button is checked
		against the run's own scope wherever it is pressed, so a name here that
		holds no grant would be a message with a button that refuses the person
		who received it.
	*/
	Notify []UserID `json:"notify,omitempty" yaml:"notify,omitempty"`
}

// Normalize trims the names and drops the empty ones, so a list written with a
// trailing item or a stray space means what it looks like.
func (p ApprovalPolicy) Normalize() ApprovalPolicy {
	out := ApprovalPolicy{Direct: p.Direct}
	seen := map[UserID]bool{}
	for _, one := range p.Notify {
		id := UserID(strings.TrimSpace(string(one)))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out.Notify = append(out.Notify, id)
	}
	return out
}

// ErrApprovalNotifyWithoutDirect means an agent named people to message while
// asking for no message. It reads as configured and does nothing.
var ErrApprovalNotifyWithoutDirect = errors.New(
	"approvals: notify names people while direct is off")

// ErrApprovalNotifyTooMany means the named list is longer than a specification
// anybody reads.
var ErrApprovalNotifyTooMany = errors.New(
	"approvals: notify names more people than one approval should reach")

/*
Validate refuses a policy that says something the platform will not do.

Naming people to message while asking for no message is the one shape worth
refusing: it reads as configured and does nothing, and an owner who wrote it
believed they had asked for something.
*/
func (p ApprovalPolicy) Validate() error {
	if !p.Direct && len(p.Notify) > 0 {
		return ErrApprovalNotifyWithoutDirect
	}
	if len(p.Notify) > MaxApprovalNotify {
		return ErrApprovalNotifyTooMany
	}
	return nil
}

// MaxApprovalNotify bounds the named list. The fan-out has its own cap on how
// many people one approval may reach; this one is about a specification staying
// a thing a person reads and understands.
const MaxApprovalNotify = 20
