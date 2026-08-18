package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
OutcomeOf reads a finished run's closing answer, from wherever that run put it.

Two eras, one reader. A run recorded before the answer moved carries it inline
and always will: the chain is immutable, and rewriting old steps to tidy this
up would break the property the product rests on. A run recorded since carries
a reference, and the bytes live where retention and erasure reach them.

An erased answer is an error rather than an empty string. "The agent finished
and said nothing" and "what it said was erased" are different facts, and a
screen that renders both as blank reports the first when the second happened.
*/
func OutcomeOf(
	ctx context.Context, store ContentStore, p domain.RunFinishedPayload,
) (string, error) {
	if p.OutcomeRef == "" {
		return p.Outcome, nil
	}
	if store == nil {
		return "", fmt.Errorf("engine: no content store to read outcome %s", p.OutcomeRef)
	}
	data, err := store.Get(ctx, p.OutcomeRef)
	if err != nil {
		if errors.Is(err, domain.ErrContentErased) {
			return "", fmt.Errorf("engine: outcome %s was erased: %w", p.OutcomeRef, err)
		}
		return "", fmt.Errorf("engine: read outcome %s: %w", p.OutcomeRef, err)
	}
	return string(data), nil
}
