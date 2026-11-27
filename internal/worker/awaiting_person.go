package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
awaitingPerson ends a run refused because a server it needs waits for a person
(#483, delivery 3b), before supervision or a fallback sees the refusal: no
backing off brings a person, another model does not either, and a parked run
would be resumed onto the same refusal. It reports whether it took the run.

Recognised by the engine's type alone. A tool call that finds its caller without
a credential carries the same code, and is supervised as it always was.
*/
func (w *Worker) awaitingPerson(ctx context.Context, claim domain.Claim, advErr error, log *slog.Logger) (bool, error) {
	var waiting *engine.AwaitingPersonError
	if !errors.As(advErr, &waiting) {
		return false, nil
	}
	summary := waiting.Summary()
	w.metrics.AdvanceFailure(summary.Code, false)
	log.Warn("a tool server waits for a person; failing the run", "server", waiting.Server)
	return true, w.failRun(ctx, claim, domain.FailedPayload{Code: summary.Code})
}
