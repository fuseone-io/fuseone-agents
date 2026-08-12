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

// Ledger is the append-only record, declared here by the consumer.
type Ledger interface {
	Append(ctx context.Context, step domain.Step) (domain.Step, error)
	RunByIdemKey(ctx context.Context, key string) (domain.RunID, error)
}

// Registry resolves an agent to the version published now.
type Registry interface {
	Versions(ctx context.Context, agent domain.AgentID) ([]domain.AgentSummary, error)
}

// Content holds what a run is about. Optional: a run can start with nothing.
type Content interface {
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error)
}

// Clock is the time a run is stamped with.
type Clock interface{ Now() time.Time }

// Pauses reports whether an agent is stopped.
//
// Checked here rather than in each trigger because every way a run can start
// goes through this one place. A pause honoured by the scheduler and not by
// the webhook is a pause that stops an agent on weekdays.
type Pauses interface {
	IsPaused(ctx context.Context, agent domain.AgentID) (bool, error)
}

// ErrPaused means the agent exists and is not running.
var ErrPaused = errors.New("trigger: the agent is paused")

// Opener turns an intention into a run.
type Opener struct {
	ledger   Ledger
	registry Registry
	content  Content
	pauses   Pauses
	clock    Clock
}

func NewOpener(ledger Ledger, registry Registry, clock Clock) *Opener {
	return &Opener{ledger: ledger, registry: registry, clock: clock}
}

// WithContent wires where the run's input is stored.
func (o *Opener) WithContent(content Content) *Opener {
	o.content = content
	return o
}

// WithPauses wires whether an agent is allowed to start at all.
func (o *Opener) WithPauses(pauses Pauses) *Opener {
	o.pauses = pauses
	return o
}

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
	// Simulated opens a run that is never claimed by a worker and never
	// counted as production. It exists because FU-10 wants a simulation
	// somebody reviewed, and reviewing one means reading its trail.
	Simulated bool
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
		Payload: mustJSON(domain.RunStartedPayload{
			Trigger: req.Trigger, InputRef: inputRef, Simulated: req.Simulated,
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
