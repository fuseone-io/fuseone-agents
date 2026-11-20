package trigger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Candidate is what a seal says about the run an experiment opens on it.
type Candidate struct {
	Agent   domain.AgentID
	Version domain.VersionID
	// Scope is the sealed scope, where the run runs even if the agent moved since.
	Scope domain.Scope
	// Paused is the agent's recorded pause. An agent with no state has nothing to pause.
	Paused bool
}

// Candidates resolves a seal of an agent, declared here by the consumer.
type Candidates interface {
	Candidate(ctx context.Context, agent domain.AgentID, seal string) (Candidate, bool, error)
}

// WithCandidates wires how an experiment's run resolves the seal it names.
func (o *Opener) WithCandidates(c Candidates) *Opener {
	o.candidates = c
	return o
}

// targetFor is what a run opens on: the candidate its seal names, or a published version.
func (o *Opener) targetFor(ctx context.Context, req Request) (domain.AgentSummary, error) {
	if req.SealID != "" {
		return o.candidateFor(ctx, req)
	}
	return o.publishedFor(ctx, req)
}

/*
candidateFor is the candidate an experiment's or a rehearsal's simulated run executes
(NT-018 phases 2 and 4).

Only an experiment's or a rehearsal's simulated run may name one. Anything else naming a seal, a seal of
another agent, a seal that holds another version than the one asked, and a seal nobody
wrote are all the error a version the agent never published gets, so a seal's id reveals
nothing. The run needs no published version and runs in the sealed scope. It is refused by
a pause the agent has (decision 3: an agent never published has nothing to pause) and by a
switch over the installation, the sealed scope or the agent.
*/
func (o *Opener) candidateFor(ctx context.Context, req Request) (domain.AgentSummary, error) {
	unknown := fmt.Errorf("%w: %s", ErrUnknownVersion, req.SealID)
	if req.Simulation == "" || (req.Experiment == "" && !req.Rehearsal) || o.candidates == nil {
		return domain.AgentSummary{}, unknown
	}
	c, found, err := o.candidates.Candidate(ctx, req.Agent, req.SealID)
	if err != nil {
		return domain.AgentSummary{}, fmt.Errorf("trigger: read seal %s: %w", req.SealID, err)
	}
	if !found || c.Agent != req.Agent || (req.Version != "" && req.Version != c.Version) {
		return domain.AgentSummary{}, unknown
	}
	if c.Paused {
		return domain.AgentSummary{}, fmt.Errorf("%w: %s", ErrPaused, req.Agent)
	}
	if err := o.stoppedIn(ctx, c.Scope, req.Agent); err != nil {
		return domain.AgentSummary{}, err
	}
	return domain.AgentSummary{ID: req.Agent, VersionID: c.Version, Scope: c.Scope}, nil
}

// stoppedIn refuses a run in scope while a switch over it is in force.
func (o *Opener) stoppedIn(ctx context.Context, scope domain.Scope, agent domain.AgentID) error {
	if o.stops == nil {
		return nil
	}
	inForce, err := o.stops.InForce(ctx)
	if err != nil {
		return fmt.Errorf("trigger: read stops: %w", err)
	}
	if stop := covering(inForce, scope, agent); stop != nil {
		return fmt.Errorf("%w: %s", ErrStopped, stop.Reason)
	}
	return nil
}
