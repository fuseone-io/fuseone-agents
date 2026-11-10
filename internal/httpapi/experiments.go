package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

/*
StartExperiment opens one simulated run per case of a dataset version, once per
repetition, on one agent version under one configuration (NT-015 § Experiments).

The same authority as a simulation, because it spends the same real money. The same
fence an erasure takes around reading the corpus and opening its runs. And never a
battery: every run names the experiment, which every door that reads batteries leaves
out.
*/
func (s *Server) StartExperiment(
	ctx context.Context, req openapi.StartExperimentRequestObject,
) (openapi.StartExperimentResponseObject, error) {
	absent := openapi.StartExperiment404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok || s.regressions == nil {
		return absent, err
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, published.Scope); err != nil {
		return openapi.StartExperiment403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, published.Scope),
		}, nil
	}
	ask, refused := askOf(req.Body)
	if refused != nil {
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(refused.Error())), nil
	}

	id := experimentID(published.ID, req.Params.IdempotencyKey)
	out, err := s.openExperiment(ctx, published, id, ask)
	switch {
	case err != nil:
		return nil, fmt.Errorf("open experiment %s: %w", id, err)
	case out.conflict != nil:
		return openapi.StartExperiment409ApplicationProblemPlusJSONResponse(conflicted(out.conflict.Error())), nil
	case out.refused != nil:
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(out.refused.Error())), nil
	case len(out.opened.Runs) == 0:
		return openapi.StartExperiment409ApplicationProblemPlusJSONResponse(
			conflicted(firstOr(out.opened.Failed, "no case opened a run"))), nil
	}
	return openapi.StartExperiment202JSONResponse{
		Id: id, Cases: out.cases, Repetitions: out.intent.Repetitions,
		Runs: len(out.opened.Runs), DatasetVersion: out.intent.DatasetVersion,
	}, nil
}

// experimentOpened is what opening an experiment came to, refusals included.
type experimentOpened struct {
	intent simulate.Intention
	cases  int
	opened simulate.Opened
	// conflict is a retry stating another intention than its key's; refused is an
	// intention nobody could run.
	conflict, refused error
}

/*
openExperiment settles the intention and opens its runs, inside the corpus fence taken
alone.

Alone, and not shared as a battery takes it, because the intention is read and then acted
on: two first attempts under one key, with different bodies, would otherwise both find
nothing recorded and open their runs into one experiment. Taken alone, the second reads
what the first recorded. The fence still keeps an erasure out while the corpus is copied.
*/
func (s *Server) openExperiment(
	ctx context.Context, published domain.AgentSummary, id string, ask experimentAsk,
) (out experimentOpened, err error) {
	err = s.regressions.Exclusive(ctx, []domain.AgentID{published.ID}, func(ctx context.Context) error {
		recorded, found, err := simulate.Recorded(ctx, s.store, id)
		if err != nil {
			return err
		}
		if out.intent, out.conflict = settle(ask, recorded, found, published.VersionID); out.conflict != nil {
			return nil
		}
		if out.refused, err = s.unservedPair(ctx, published, out.intent); err != nil || out.refused != nil {
			return err
		}
		corpus, readErr := s.corpusOccurrences(ctx, published.ID, out.intent.DatasetVersion)
		if out.refused = readErr; readErr != nil {
			return nil
		}
		out.intent.DatasetVersion, out.cases = corpus.Version, len(corpus.Cases)
		out.opened, err = simulate.Open(ctx, s.opener(), simulate.Batch{
			ID: id, Agent: published.ID, By: callerOf(ctx),
			DatasetVersion: corpus.Version, Cases: corpus.Cases, Experiment: &out.intent.Experiment,
		})
		return err
	})
	return out, err
}

// experimentID names an experiment after its agent and the intention alone, so asking
// again with the same key reaches the runs it already opened.
func experimentID(agent domain.AgentID, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("exp_%s_%s", agent, hex.EncodeToString(sum[:8]))
}
