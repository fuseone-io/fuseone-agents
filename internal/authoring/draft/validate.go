package draft

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/spec"
)

// MaxDefinitionBytes is the most a draft's rendered definition may hold: the
// ceiling a published file has, so a draft never holds what could not be
// published, and never anything the content store would truncate.
const MaxDefinitionBytes = 2 << 20

var (
	// ErrRefused is an operation that leaves a draft publication would refuse
	// for being wrong. The error says why, in the publication's words.
	ErrRefused = errors.New("draft: publication would refuse this")
	// ErrTooLarge is a draft over MaxDefinitionBytes, refused before it is
	// written and never truncated.
	ErrTooLarge = errors.New("draft: the definition is larger than a draft may hold")
)

// Validator judges operations: the shared checks of every publication.
type Validator struct {
	Checker publishable.Checker
}

// Applied is a draft an operation left, and what it still lacks.
type Applied struct {
	Draft   Draft
	Missing []spec.Missing
}

/*
Validate applies an operation to a copy of the draft and judges what it left
(NT-018, contract 6).

The copy is taken through the canonical form, spec.Render then ParseDraft,
and then through the checks a publication runs: providers configured for the
scope, tools offered there, schedules this platform reads, no untrusted data
reaching a non-reversible tool. So an accepted operation never leaves a draft
publication would refuse for being wrong. What is only unfinished is
returned in Missing. now is the instant schedules are read from.
*/
func (v Validator) Validate(ctx context.Context, d Draft, op Operation, origin Origin, now time.Time) (Applied, error) {
	if err := admitted(op, origin); err != nil {
		return Applied{}, err
	}
	next := d.clone()
	if err := op.apply(&next); err != nil {
		return Applied{}, err
	}
	if scope := publishable.ScopeOf(next.Spec); !scope.Valid() {
		return Applied{}, fmt.Errorf("%w: a draft names its company and its area, and this one is %s", ErrRefused, scope)
	}
	rendered, err := spec.Render(next.Definition())
	if err != nil {
		return Applied{}, fmt.Errorf("draft: render: %w", err)
	}
	if len(rendered) > MaxDefinitionBytes {
		return Applied{}, fmt.Errorf("%w: %d bytes, at most %d", ErrTooLarge, len(rendered), MaxDefinitionBytes)
	}
	canonical, missing, err := spec.ParseDraft("draft", rendered)
	if err != nil {
		return Applied{}, fmt.Errorf("%w: %v", ErrRefused, err)
	}
	detail, err := v.Checker.Check(ctx, canonical, now)
	if err != nil {
		return Applied{}, err
	}
	if detail != "" {
		return Applied{}, fmt.Errorf("%w: %s", ErrRefused, detail)
	}
	return Applied{Draft: next, Missing: missing}, nil
}
