package trigger

import (
	"context"
	"fmt"
	"regexp"

	"github.com/fuseone/agents/internal/domain"
)

// Threads resolves the conversation a rehearsal belongs to, declared here by the consumer.
type Threads interface {
	ThreadFor(ctx context.Context, agent domain.AgentID, draftID string, author domain.UserID) (string, error)
}

// WithThreads wires how a rehearsal finds its author's conversation.
func (o *Opener) WithThreads(t Threads) *Opener {
	o.threads = t
	return o
}

// threadShape is a conversation's id as the drafts store writes it: a UUID.
var threadShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

/*
rehearsalShape refuses a rehearsal mark on a request that is not exactly a rehearsal: a
simulated run of a candidate, for a person whose conversation it joins, and never an
experiment at the same time. An experiment's readers would count it, and a rehearsal's
answers would pose as the experiment's measurements.
*/
func rehearsalShape(req Request) error {
	if !req.Rehearsal {
		return nil
	}
	if req.Simulation == "" || req.SealID == "" || req.Experiment != "" || req.By == "" {
		return fmt.Errorf("%w: %s", ErrRehearsalOutsideCandidate, req.IdemKey)
	}
	return nil
}

/*
threadOf is the conversation a rehearsal belongs to: its author's on the draft the seal was
sealed from, resolved here and never taken from the caller.

Called after every refusal and before anything is written, because resolving it opens the
author's thread the first time: a rehearsal that does not open leaves no thread behind. A
rehearsal with no conversation does not open at all, since erasing the conversation is what
reaches the case its author typed.
*/
func (o *Opener) threadOf(ctx context.Context, req Request, draft string) (string, error) {
	if !req.Rehearsal {
		return "", nil
	}
	if o.threads == nil || draft == "" {
		return "", fmt.Errorf("trigger: rehearsal %s has no conversation to belong to", req.IdemKey)
	}
	thread, err := o.threads.ThreadFor(ctx, req.Agent, draft, req.By)
	if err != nil {
		return "", fmt.Errorf("trigger: the conversation of rehearsal %s: %w", req.IdemKey, err)
	}
	if !threadShape.MatchString(thread) {
		return "", fmt.Errorf("trigger: rehearsal %s was given %q as its conversation", req.IdemKey, thread)
	}
	return thread, nil
}
