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
// Request is one intention to run an agent.
type Request struct {
	Agent domain.AgentID
	// IdemKey names the intention, not the attempt. Two requests carrying the
	// same key are the same intention however far apart they arrive.
	IdemKey string
	// Trigger is what caused it: manual, cron, webhook, event. It is recorded
	// because "why did this run" is the first question asked about any run
	// nobody remembers starting.
	Trigger string
	// By is who asked, where a person did.
	By domain.UserID
	// Input is what the run is about. Stored outside the ledger.
	Input []byte
	/*
		Labels are what is true about where the input came from.

		Sealed on `run_started`, which the fold unions into the run, so a run
		opened from outside meets the taint check on its very first proposal
		rather than only after an untrusted tool has answered. Without it an
		agent that read a webhook body and wrote straight from it passed the
		check that exists for exactly that.

		A fact about provenance and never a posture: a run the clock opened
		carries none, because nobody outside said anything to it.
	*/
	Labels domain.Labels
	// Origin is the conversation this ask arrived in, when one did. Sealed on
	// the opening step, which is what bounds where a reply may go: the run
	// answers where it was asked, and nowhere else is a decision the platform
	// would be making.
	Origin *domain.RunOrigin
	// Simulation names the batch this run belongs to, and opening a run under
	// one is what marks it simulated: never claimed by a worker, never
	// counted as production. One field rather than a name beside a flag,
	// because the failure a flag allows is a run marked as a simulation that
	// a worker claims and executes for real.
	Simulation string
	// Case names the regression case being replayed, when one is.
	Case string
}

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

	if existing, err := o.ledger.RunByIdemKey(ctx, req.IdemKey); err == nil {
		return Result{RunID: existing}, nil
	}

	// After the key and before anything is written: a repeat of an intention
	// that already opened a run still answers with that run, because pausing
	// an agent does not unmake what it already started.
	if o.pauses != nil {
		stopped, err := o.pauses.IsPaused(ctx, req.Agent)
		if err != nil {
			return Result{}, fmt.Errorf("trigger: read pause state: %w", err)
		}
		if stopped {
			return Result{}, fmt.Errorf("%w: %s", ErrPaused, req.Agent)
		}
	}

	// Wider than the agent, and checked after it: an operator who has stopped
	// a scope gets one refusal naming the scope, rather than a per-agent
	// message that makes them wonder which agents are covered.
	//
	// A simulation is stopped too. It spends real money at the provider and
	// the person who pressed this wanted the platform quiet.
	if o.stops != nil {
		stop, err := o.stopping(ctx, req.Agent)
		if err != nil {
			return Result{}, err
		}
		if stop != nil {
			return Result{}, fmt.Errorf("%w: %s", ErrStopped, stop.Reason)
		}
	}

	// A draft may be simulated and may not act. That is what Draft means, and
	// it is checked here rather than at the Gate because a run that should
	// never have opened leaves a trail somebody has to explain.
	if o.stages != nil && req.Simulation == "" {
		stage, err := o.stages.StageOf(ctx, req.Agent)
		if err != nil {
			return Result{}, fmt.Errorf("trigger: read stage: %w", err)
		}
		if stage == domain.StageDraft {
			return Result{}, fmt.Errorf("%w: %s", ErrDraft, req.Agent)
		}
	}

	versions, err := o.registry.Versions(ctx, req.Agent)
	if err != nil {
		return Result{}, fmt.Errorf("trigger: versions of %s: %w", req.Agent, err)
	}
	if len(versions) == 0 {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownAgent, req.Agent)
	}
	published := versions[0]

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
		Labels:     req.Labels,
		Payload: mustJSON(domain.RunStartedPayload{
			Trigger: req.Trigger, InputRef: inputRef,
			Simulated: req.Simulation != "", Simulation: req.Simulation,
			Case: req.Case, Origin: req.Origin,
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
