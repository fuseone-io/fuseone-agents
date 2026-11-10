package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

// experimentAsk is what a request states. Nil is a field it leaves to the experiment: the
// first attempt's when there was one, the default when this is the first.
type experimentAsk struct {
	version        *domain.VersionID
	config         *domain.ExperimentConfig
	repetitions    *int
	datasetVersion *int64
}

// askOf reads a request, refusing what no experiment could run.
func askOf(body *openapi.ExperimentRequest) (experimentAsk, error) {
	var out experimentAsk
	if body == nil {
		return out, nil
	}
	if r := body.Repetitions; r != nil && (*r < 1 || *r > simulate.MaxRepetitions) {
		return out, fmt.Errorf("an experiment runs each case 1 to %d times", simulate.MaxRepetitions)
	}
	out.repetitions = body.Repetitions
	if body.Version != nil {
		v := domain.VersionID(*body.Version)
		out.version = &v
	}
	if c := body.Config; c != nil {
		out.config = &domain.ExperimentConfig{
			Provider: deref(c.Provider), Model: deref(c.Model), Effort: deref(c.Effort),
		}
	}
	// Zero is how the schema says "the latest", so it is not a version anybody stated.
	if d := body.DatasetVersion; d != nil && *d != 0 {
		out.datasetVersion = d
	}
	return out, nil
}

/*
fresh is the intention of a first attempt: what it states, and otherwise one case once,
as published, on the version published now and the dataset's latest.

The version is pinned here and not left empty. Empty asks every run to resolve "published
now" on its own, and a publication between two of them puts one experiment on two
versions. The dataset needs no pin here: it is read once, and that read is the pin.
*/
func fresh(ask experimentAsk, published domain.VersionID) simulate.Intention {
	out := simulate.Intention{Experiment: simulate.Experiment{Version: published, Repetitions: 1}}
	if ask.version != nil {
		out.Version = *ask.version
	}
	if ask.config != nil {
		out.Config = *ask.config
	}
	if ask.repetitions != nil {
		out.Repetitions = *ask.repetitions
	}
	if ask.datasetVersion != nil {
		out.DatasetVersion = *ask.datasetVersion
	}
	return out
}

// diverging names what a retry states differently from what its first attempt recorded.
func diverging(ask experimentAsk, recorded simulate.Intention) []string {
	var out []string
	if ask.version != nil && *ask.version != recorded.Version {
		out = append(out, fmt.Sprintf("agent version %s, not %s", recorded.Version, *ask.version))
	}
	if ask.config != nil && *ask.config != recorded.Config {
		out = append(out, "another configuration")
	}
	if ask.repetitions != nil && *ask.repetitions != recorded.Repetitions {
		out = append(out, fmt.Sprintf("%d repetitions, not %d", recorded.Repetitions, *ask.repetitions))
	}
	if ask.datasetVersion != nil && *ask.datasetVersion != recorded.DatasetVersion {
		out = append(out, fmt.Sprintf("dataset version %d, not %d", recorded.DatasetVersion, *ask.datasetVersion))
	}
	return out
}

// settle is the intention this request runs: the first attempt's whole, when there was
// one and the request states nothing else, and a fresh one otherwise.
func settle(
	ask experimentAsk, recorded simulate.Intention, found bool, published domain.VersionID,
) (simulate.Intention, error) {
	if !found {
		return fresh(ask, published), nil
	}
	if differs := diverging(ask, recorded); len(differs) > 0 {
		return simulate.Intention{}, fmt.Errorf(
			"this idempotency key already names an experiment with %s", strings.Join(differs, ", "))
	}
	return recorded, nil
}

/*
unservedPair refuses an intention whose runs could only park: the pair they would plan on,
the configuration over the version field by field, is not one this installation could ask.

Asked of the providers this process built, not of every name configured: one switched off,
or whose credential did not open, answers nothing. A version that does not exist is left to
the opener, which refuses it and says so. Unwired, there is nothing to ask.
*/
func (s *Server) unservedPair(
	ctx context.Context, published domain.AgentSummary, intent simulate.Intention,
) (refused error, err error) {
	if s.providerNames == nil {
		return nil, nil
	}
	version, found := published, intent.Version == published.VersionID
	if !found {
		if version, found, err = s.versionOf(ctx, published.ID, intent.Version); err != nil || !found {
			return nil, err
		}
	}
	provider := version.Provider
	if intent.Config.Provider != "" {
		provider = intent.Config.Provider
	}
	if s.providerNames.Usable(provider) {
		return nil, nil
	}
	return fmt.Errorf("the experiment would run on provider %q, which this installation cannot ask",
		provider), nil
}

func (s *Server) versionOf(
	ctx context.Context, agent domain.AgentID, id domain.VersionID,
) (domain.AgentSummary, bool, error) {
	versions, err := s.agents.Versions(ctx, agent)
	if err != nil {
		return domain.AgentSummary{}, false, fmt.Errorf("versions of %s: %w", agent, err)
	}
	for _, v := range versions {
		if v.VersionID == id {
			return v, true, nil
		}
	}
	return domain.AgentSummary{}, false, nil
}
