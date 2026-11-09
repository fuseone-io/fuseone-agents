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
	experiment, asked, refused := s.experimentOf(req.Body)
	if refused != nil {
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(refused.Error())), nil
	}

	id := experimentID(published.ID, req.Params.IdempotencyKey)
	corpus, opened, refused, err := s.openExperiment(ctx, published.ID, id, asked, experiment)
	switch {
	case err != nil:
		return nil, fmt.Errorf("open experiment %s: %w", id, err)
	case refused != nil:
		return openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(refused.Error())), nil
	case len(opened.Runs) == 0:
		return openapi.StartExperiment409ApplicationProblemPlusJSONResponse(
			conflicted(firstOr(opened.Failed, "no case opened a run"))), nil
	}
	return openapi.StartExperiment202JSONResponse{
		Id: id, Cases: len(corpus.Cases), Repetitions: experiment.Repetitions,
		Runs: len(opened.Runs), DatasetVersion: corpus.Version,
	}, nil
}

// openExperiment reads the dataset version and opens its runs inside the erasure fence.
func (s *Server) openExperiment(
	ctx context.Context, agent domain.AgentID, id string, asked int64, experiment simulate.Experiment,
) (corpus simulate.Corpus, opened simulate.Opened, refused error, err error) {
	err = s.regressions.Hold(ctx, agent, func(ctx context.Context) error {
		var readErr error
		if corpus, readErr = s.corpusOccurrences(ctx, agent, asked); readErr != nil {
			refused = readErr
			return nil
		}
		var openErr error
		opened, openErr = simulate.Open(ctx, s.opener(), simulate.Batch{
			ID: id, Agent: agent, By: callerOf(ctx),
			DatasetVersion: corpus.Version, Cases: corpus.Cases, Experiment: &experiment,
		})
		return openErr
	})
	return corpus, opened, refused, err
}

// experimentOf is what the request asks for, refused when it cannot be run as asked.
func (s *Server) experimentOf(body *openapi.ExperimentRequest) (simulate.Experiment, int64, error) {
	out := simulate.Experiment{Repetitions: 1}
	if body == nil {
		return out, 0, nil
	}
	if body.Repetitions != nil {
		out.Repetitions = *body.Repetitions
	}
	if out.Repetitions < 1 || out.Repetitions > simulate.MaxRepetitions {
		return out, 0, fmt.Errorf("an experiment runs each case 1 to %d times", simulate.MaxRepetitions)
	}
	if body.Version != nil {
		out.Version = domain.VersionID(*body.Version)
	}
	if body.Config != nil {
		out.Config = domain.ExperimentConfig{
			Provider: deref(body.Config.Provider), Model: deref(body.Config.Model), Effort: deref(body.Config.Effort),
		}
	}
	if p := out.Config.Provider; p != "" && s.providerNames != nil && !s.providerNames.Configured(p) {
		return out, 0, fmt.Errorf("provider %q is not configured on this installation", p)
	}
	var asked int64
	if body.DatasetVersion != nil {
		asked = *body.DatasetVersion
	}
	return out, asked, nil
}

// experimentID names an experiment after its agent and the intention alone, so asking
// again with the same key reaches the runs it already opened.
func experimentID(agent domain.AgentID, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("exp_%s_%s", agent, hex.EncodeToString(sum[:8]))
}
