// Package trigger opens runs.
//
// Every way a run can start — a person pressing a button, a schedule coming
// due, a webhook arriving — ends in the same place: one appended step, pinned
// to the version published at that moment, carrying an idempotency key that
// says which intention it belongs to.
//
// One place on purpose. Two paths that both "just append run_started" drift,
// and the way they drift is that one of them forgets the key and starts
// opening a run every time it is retried.
package trigger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Opening a run: the first step of the chain, and the last chance to refuse
before one exists.

A stop, a pause and an unpublished agent all end here rather than inside the
engine, because a run that was opened only to be killed on its first turn
still costs a row, a hash and an entry somebody has to read.
*/
// ErrRehearsalOutsideCandidate is a rehearsal mark on a request that is not a simulated run
// of a candidate. Dropping the mark would open an ordinary run where the caller asked for
// one whose calls reach nothing.
var ErrRehearsalOutsideCandidate = errors.New("trigger: a rehearsal runs a candidate, in a simulation")

// ErrUnknownAgent means nothing is published under that id.
var ErrUnknownAgent = fmt.Errorf("trigger: no published version")

// Result says which run the intention names, and whether this call opened it.
type Result struct {
	RunID   domain.RunID
	Scope   domain.Scope
	Created bool
}

// Open opens the run, or reports the one this key already opened.
//
// The key answers first, so a caller retrying after a timeout reaches the run
// it started rather than starting another. A run is real tools against real
// systems; opening a second one because a response was lost is the failure
// this exists to prevent.
func (o *Opener) Open(ctx context.Context, req Request) (Result, error) {
	if req.IdemKey == "" {
		return Result{}, fmt.Errorf("trigger: an intention needs a key")
	}
	ticket := cloneTicket(req.Ticket)
	if ticket != nil && !ticket.Valid() {
		return Result{}, fmt.Errorf("trigger: invalid ticket context")
	}
	workItem, err := workItemOf(req)
	if err != nil {
		return Result{}, err
	}
	experimentConfig, err := experimentOf(req)
	if err != nil {
		return Result{}, err
	}
	if err := rehearsalShape(req); err != nil {
		return Result{}, err
	}

	if existing, err := o.ledger.RunByIdemKey(ctx, req.IdemKey); err == nil {
		return Result{RunID: existing}, nil
	}

	opening, err := o.targetFor(ctx, req)
	if err != nil {
		return Result{}, err
	}
	published := opening.summary
	if violation, blocked := req.Labels.ScopeBoundaryViolation(published.Scope); blocked {
		return Result{}, fmt.Errorf("trigger: %w", violation)
	}
	for _, artifact := range req.ContextArtifacts {
		if violation, blocked := artifact.Labels.ScopeBoundaryViolation(published.Scope); blocked {
			return Result{}, fmt.Errorf("trigger: %w", violation)
		}
	}
	labels := req.Labels.Union(domain.ScopeLabels(published.Scope))
	thread, err := o.threadOf(ctx, req, opening.draft)
	if err != nil {
		return Result{}, err
	}

	runID := newRunID(published.ID, o.clock.Now(), req.IdemKey)

	inputRef, err := o.store(ctx, runID, req.Input)
	if err != nil {
		return Result{}, err
	}

	_, err = o.ledger.Append(ctx, domain.Step{
		RunID:      runID,
		Kind:       domain.StepRunStarted,
		Scope:      published.Scope,
		AgentID:    published.ID,
		VersionID:  published.VersionID,
		OnBehalfOf: req.By,
		IdemKey:    req.IdemKey,
		At:         o.clock.Now(),
		Labels:     labels,
		Payload: mustJSON(domain.RunStartedPayload{
			Trigger: req.Trigger, InputRef: inputRef,
			Simulated: req.Simulation != "", Simulation: req.Simulation,
			Case: req.Case, DatasetVersion: req.DatasetVersion, Origin: req.Origin, Ticket: ticket,
			ContextArtifacts: cloneContextArtifacts(req.ContextArtifacts),
			SourceRun:        req.SourceRun,
			Chain:            req.Chain,
			WorkItem:         workItem,
			Experiment:       req.Experiment,
			ExperimentConfig: experimentConfig,
			SealID:           req.SealID,
			Rehearsal:        req.Rehearsal,
			AuthoringThread:  thread,

			ExperimentRepetitions: req.ExperimentRepetitions,
		}),
	})
	if err != nil {
		// Two attempts raced and this one lost. Asked rather than matched on
		// the error: whatever went wrong, if the key now names a run then that
		// run is the answer, and if it does not the failure was real.
		if existing, lookupErr := o.ledger.RunByIdemKey(ctx, req.IdemKey); lookupErr == nil {
			return Result{RunID: existing, Scope: published.Scope}, nil
		}
		return Result{}, fmt.Errorf("trigger: open run: %w", err)
	}

	return Result{RunID: runID, Scope: published.Scope, Created: true}, nil
}

// refusedToStart is why the agent may not start now: paused, or under a switch wider than
// it (PRD FO-06).
func (o *Opener) refusedToStart(ctx context.Context, agent domain.AgentID) error {
	// After the key and before anything is written: a repeat of an intention
	// that already opened a run still answers with that run, because pausing
	// an agent does not unmake what it already started.
	if o.pauses != nil {
		stopped, err := o.pauses.IsPaused(ctx, agent)
		if err != nil {
			return fmt.Errorf("trigger: read pause state: %w", err)
		}
		if stopped {
			return fmt.Errorf("%w: %s", ErrPaused, agent)
		}
	}

	// Wider than the agent, and checked after it: an operator who has stopped
	// a scope gets one refusal naming the scope, rather than a per-agent
	// message that makes them wonder which agents are covered.
	//
	// A simulation is stopped too. It spends real money at the provider and
	// the person who pressed this wanted the platform quiet.
	if o.stops != nil {
		stop, err := o.stopping(ctx, agent)
		if err != nil {
			return err
		}
		if stop != nil {
			return fmt.Errorf("%w: %s", ErrStopped, stop.Reason)
		}
	}
	return nil
}

// publishedFor is the published version a run opens on: refused while the agent is paused
// or a wider switch is off, refused for a real run of a draft, and pinned to the version
// the request named or the one published now.
func (o *Opener) publishedFor(ctx context.Context, req Request) (domain.AgentSummary, error) {
	if err := o.refusedToStart(ctx, req.Agent); err != nil {
		return domain.AgentSummary{}, err
	}

	// A draft may be simulated and may not act. That is what Draft means, and
	// it is checked here rather than at the Gate because a run that should
	// never have opened leaves a trail somebody has to explain.
	if o.stages != nil && req.Simulation == "" {
		stage, err := o.stages.StageOf(ctx, req.Agent)
		if err != nil {
			return domain.AgentSummary{}, fmt.Errorf("trigger: read stage: %w", err)
		}
		if stage == domain.StageDraft {
			return domain.AgentSummary{}, fmt.Errorf("%w: %s", ErrDraft, req.Agent)
		}
	}

	versions, err := o.registry.Versions(ctx, req.Agent)
	if err != nil {
		return domain.AgentSummary{}, fmt.Errorf("trigger: versions of %s: %w", req.Agent, err)
	}
	if len(versions) == 0 {
		return domain.AgentSummary{}, fmt.Errorf("%w: %s", ErrUnknownAgent, req.Agent)
	}
	return Pin(versions, req.Version)
}

// store puts what the run is about outside the ledger. A ticket or a message
// routinely carries personal data, and the ledger is kept for years (AU-04).
func (o *Opener) store(ctx context.Context, runID domain.RunID, input []byte) (string, error) {
	if len(input) == 0 || o.content == nil {
		return "", nil
	}
	ref, err := o.content.Put(ctx, runID, domain.FirstSeq, input)
	if err != nil {
		return "", fmt.Errorf("trigger: store input: %w", err)
	}
	return ref, nil
}

// newRunID names a run after the intention that opened it.
//
// The key is folded in because a timestamp is not unique: two webhooks in the
// same millisecond derive the same id, and the second run_started is appended
// as the next step of the first run rather than rejected — two intentions
// silently become one run. Derived rather than random, so the id can be
// recomputed from the intention by anybody auditing it.
func newRunID(agent domain.AgentID, at time.Time, key string) domain.RunID {
	sum := sha256.Sum256([]byte(key))
	return domain.RunID(fmt.Sprintf(
		"run_%s_%d_%s", agent, at.UnixMilli(), hex.EncodeToString(sum[:4])))
}
