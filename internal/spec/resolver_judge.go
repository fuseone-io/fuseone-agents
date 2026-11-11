package spec

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/judge"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/worker"
)

// Content is what a judge run's material is read from, declared here by the consumer.
type Content interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

/*
Judges resolves judge runs (NT-016): the rubric as the system prompt, the case and the
judged output as the only user turn, an empty pack, the judgment's ceiling and the judge's
pair. Nothing of the version being judged is planned with, not even its instructions.
*/
type Judges struct {
	resolver *Resolver
	content  Content
}

var _ worker.Judges = (*Judges)(nil)

// Judges is this resolver's providers, reading judge runs' material from content.
func (r *Resolver) Judges(content Content) *Judges {
	return &Judges{resolver: r, content: content}
}

// ResolveJudge is what a judge run plans with, refused when any object its opening names
// does not match its digest.
func (j *Judges) ResolveJudge(
	ctx context.Context, run domain.RunID, started domain.RunStartedPayload,
) (worker.Resolution, error) {
	material, err := readJudgeMaterial(ctx, j.content, started)
	if err != nil {
		return worker.Resolution{}, fmt.Errorf("judge %s: %w", run, err)
	}
	turn, err := judge.UserTurn(judge.Boundary(material.salt, run), material.input, material.output)
	if err != nil {
		return worker.Resolution{}, fmt.Errorf("judge %s: %w", run, err)
	}
	opening := started.Judge
	planner, err := j.resolver.providers.Planner(opening.Provider, model.Config{
		Model: opening.Model, Effort: opening.Effort,
		SystemPrompt: judge.SystemPrompt(material.rubric),
		// One cache identity per rubric: two judgments under it share their prefix.
		CacheKey: "judge:" + opening.RubricDigest,
	}, j.resolver.tools)
	if err != nil {
		return worker.Resolution{}, fmt.Errorf("judge %s on %s/%s: %w", run, opening.Provider, opening.Model, err)
	}
	return worker.Resolution{
		Start: engine.Start{
			Pack:    gate.NewPack(),
			Budget:  domain.Budget{Micros: opening.CeilingMicros},
			Trigger: "worker",
			Closer:  verdictCloser{rubric: material.rubric, salt: material.salt},
		},
		Planner: blocksPlanner{inner: planner, turn: turn},
	}, nil
}
