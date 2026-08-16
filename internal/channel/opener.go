package channel

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/trigger"
)

/*
Opening a run, and telling a decline apart from a failure.

The consumer declares what it needs of an opener and this satisfies it, which
is the only place both contracts are known. An agent that is paused, stopped by
a switch or still a draft *declined*: it will be just as paused next sweep, so
the person is told and the ask is closed. A ledger that was away is not that,
and an ask closed because of it would answer a good question with a refusal
that was never about the question.

An earlier comment claimed the packages could not import each other. They can —
neither imports the other today, and the check was never made. The interface
stays anyway, for the reason interfaces are declared by consumers here: it is
what keeps the consumer's own tests free of a ledger.
*/

// FromTrigger adapts the platform's opener to what a consumer needs.
func FromTrigger(opener *trigger.Opener) Opens { return openerFor{opener} }

type openerFor struct{ opener *trigger.Opener }

func (o openerFor) Open(ctx context.Context, req Request) (Opened, error) {
	opened, err := o.opener.Open(ctx, trigger.Request{
		Agent:   req.Agent,
		IdemKey: req.IdemKey,
		Trigger: req.Trigger,
		By:      req.By,
		Input:   req.Input,
		Labels:  req.Labels,
		Origin:  req.Origin,
	})
	if declined(err) {
		// Wrapped rather than replaced: the sentence the trigger wrote is what
		// the person is told, and it already says which of the four it was.
		return Opened{}, fmt.Errorf("%w: %w", ErrWontStart, err)
	}
	if err != nil {
		return Opened{}, err
	}
	return Opened{RunID: opened.RunID, Created: opened.Created}, nil
}

/*
declined reports whether the opener said no rather than failed.

Four states and one answer, because the ask's future is the same in all of
them: it will not start now and it will not start on a retry, so the honest
thing is to close it and say so. An agent nobody published is here for the same
reason — a name that resolves to nothing will resolve to nothing again.
*/
func declined(err error) bool {
	return errors.Is(err, trigger.ErrPaused) ||
		errors.Is(err, trigger.ErrStopped) ||
		errors.Is(err, trigger.ErrDraft) ||
		errors.Is(err, trigger.ErrUnknownAgent)
}
