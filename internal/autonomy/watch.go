/*
Package autonomy moves an agent between stages on what people decided.

Two directions, and they are deliberately not symmetric.

Promotion is only ever suggested. An agent that promoted itself for agreeing
with people is an agent that stops being asked about, and this whole product is
an argument that a person stays in the loop (FU-14).

Demotion happens. An agent whose decisions people keep refusing is doing damage
between the moment somebody notices and the moment they get round to it, and
waiting for a human to confirm what the trail already says is the expensive
mistake (FU-15).
*/
package autonomy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Agreements is what people decided, declared here by the consumer.
type Agreements interface {
	Agreement(ctx context.Context, since time.Time) ([]domain.Agreement, error)
}

// Stages is where an agent's trust is kept.
type Stages interface {
	Stages(ctx context.Context) (map[domain.AgentID]domain.Stage, error)
	SetStage(ctx context.Context, agent domain.AgentID, stage domain.Stage, by domain.UserID) error
}

// Recorder writes what the platform decided to the administrative trail.
type Recorder interface {
	Record(ctx context.Context, agent domain.AgentID, action string, detail any) error
}

// Window is how far back agreement is read.
//
// Bounded because a rate over all of history never recovers from a bad first
// week: an agent that improved would stay demoted for ever, and nobody could
// tell whether the number described the agent or its past.
const Window = 30 * 24 * time.Hour

// Watch demotes agents people have stopped agreeing with.
type Watch struct {
	agreements Agreements
	stages     Stages
	trail      Recorder
	log        *slog.Logger
}

func New(agreements Agreements, stages Stages, trail Recorder, log *slog.Logger) *Watch {
	if log == nil {
		log = slog.Default()
	}
	return &Watch{agreements: agreements, stages: stages, trail: trail, log: log}
}

/*
Sweep demotes every autonomous agent being overruled too often.

Only from autonomous, and only ever one step: to copilot, where the agent keeps
working and a person sees each action. Dropping it to draft would stop the work
outright over a rate somebody may be about to explain, and a platform that
switches an agent off on a statistic is a platform nobody leaves switched on.
*/
func (w *Watch) Sweep(ctx context.Context, now time.Time) (int, error) {
	agreements, stages, err := w.read(ctx, now)
	if err != nil {
		return 0, err
	}

	demoted := 0
	for _, agreement := range agreements {
		if stages[agreement.Agent] != domain.StageAutonomous || !agreement.WarrantsDemotion() {
			continue
		}
		if err := w.stages.SetStage(ctx, agreement.Agent, domain.StageCopilot, "platform"); err != nil {
			// Logged and left for the next sweep. One agent that could not be
			// demoted must not stop the others being.
			w.log.Error("could not demote", "agent", agreement.Agent, "err", err)
			continue
		}
		demoted++

		w.log.Warn("agent demoted to copilot", "agent", agreement.Agent,
			"approved", agreement.Approved, "refused", agreement.Refused)
		w.record(ctx, agreement)
	}
	return demoted, nil
}

// Suggestions reports the agents worth promoting, and never promotes one.
func (w *Watch) Suggestions(ctx context.Context, now time.Time) ([]domain.Agreement, error) {
	agreements, stages, err := w.read(ctx, now)
	if err != nil {
		return nil, err
	}

	var out []domain.Agreement
	for _, agreement := range agreements {
		// Only a copilot is promoted by agreement. A draft has not been
		// simulated and reviewed yet, and that gate is a person's too (FU-10).
		if stages[agreement.Agent] == domain.StageCopilot && agreement.SuggestsPromotion() {
			out = append(out, agreement)
		}
	}
	return out, nil
}

func (w *Watch) read(
	ctx context.Context, now time.Time,
) ([]domain.Agreement, map[domain.AgentID]domain.Stage, error) {
	agreements, err := w.agreements.Agreement(ctx, now.Add(-Window))
	if err != nil {
		return nil, nil, fmt.Errorf("autonomy: read agreement: %w", err)
	}
	stages, err := w.stages.Stages(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("autonomy: read stages: %w", err)
	}
	return agreements, stages, nil
}

// record puts a demotion in the trail. The platform acting on its own is
// exactly the change somebody comes looking for an explanation of.
func (w *Watch) record(ctx context.Context, agreement domain.Agreement) {
	if w.trail == nil {
		return
	}
	if err := w.trail.Record(ctx, agreement.Agent, "agent.demoted", map[string]any{
		"from": domain.StageAutonomous, "to": domain.StageCopilot,
		"approved": agreement.Approved, "refused": agreement.Refused,
		"windowDays": int(Window.Hours() / 24),
	}); err != nil {
		w.log.Error("could not record a demotion", "agent", agreement.Agent, "err", err)
	}
}
