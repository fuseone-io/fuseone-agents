package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

/*
openSimulation reads the occurrences and opens their runs.

A corpus does both inside the fence an erasure takes: read before an erasure and
opened after it, these runs would hold what was just erased. An uploaded set has
no run to be erased from, so it needs no fence.
*/
func (s *Server) openSimulation(
	ctx context.Context, agent domain.AgentID, id string, body *openapi.SimulationRequest,
) (opened simulate.Opened, refused error, err error) {
	open := func(ctx context.Context) error {
		corpus, readErr := s.occurrences(ctx, agent, id, body)
		if readErr != nil {
			refused = readErr
			return nil
		}
		var openErr error
		opened, openErr = simulate.Open(ctx, s.opener(), simulate.Batch{
			ID: id, Agent: agent, By: callerOf(ctx),
			DatasetVersion: corpus.Version, Cases: corpus.Cases,
		})
		return openErr
	}
	if body.Corpus != nil && *body.Corpus && s.regressions != nil {
		err = s.regressions.Hold(ctx, agent, open)
	} else {
		err = open(ctx)
	}
	return opened, refused, err
}

/*
occurrences is what the simulation will replay: an uploaded set, or the corpus.

Replaying the corpus is the battery, and it reads each case back from the
claim check rather than from the run it was corrected from — that copy is the
whole reason the corpus survives retention.
*/
func (s *Server) occurrences(
	ctx context.Context, agent domain.AgentID, simulation string,
	body *openapi.SimulationRequest,
) (simulate.Corpus, error) {
	if body.Corpus != nil && *body.Corpus {
		return s.corpusOccurrences(ctx, agent, 0)
	}

	file := ""
	if body.Cases != nil {
		file = *body.Cases
	}
	cases, err := simulate.Load(ctx, s.cases, simulation, []byte(file))
	if err != nil {
		return simulate.Corpus{}, err
	}
	out := make([]simulate.Occurrence, 0, len(cases))
	for _, input := range cases {
		out = append(out, simulate.Occurrence{Input: input})
	}
	return simulate.Corpus{Cases: out}, nil
}

// corpusOccurrences reads the cases of one version of the corpus, the latest when the
// version asked is zero.
func (s *Server) corpusOccurrences(
	ctx context.Context, agent domain.AgentID, asked int64,
) (simulate.Corpus, error) {
	if s.regressions == nil {
		return simulate.Corpus{}, fmt.Errorf("this installation keeps no corrections")
	}
	version, corpus, err := s.corpusAt(ctx, agent, asked)
	if err != nil {
		return simulate.Corpus{}, err
	}
	if len(corpus) == 0 {
		return simulate.Corpus{}, fmt.Errorf("%s has no corrections to re-check", agent)
	}

	out := simulate.Corpus{Version: version, Cases: make([]simulate.Occurrence, 0, len(corpus))}
	for _, c := range corpus {
		// Erased is a stated gap, and the report names it. Anything else refuses
		// the whole set: a battery missing a case for no reason reports green on
		// a correction nobody checked.
		if c.Erased {
			continue
		}
		input, err := s.content.Get(ctx, c.InputRef)
		if errors.Is(err, domain.ErrContentErased) {
			continue
		}
		if err != nil {
			return simulate.Corpus{}, fmt.Errorf("the occurrence of %s could not be read: %w", c.ID, err)
		}
		out.Cases = append(out.Cases, simulate.Occurrence{ID: c.ID, Input: input})
	}
	if len(out.Cases) == 0 {
		return simulate.Corpus{}, fmt.Errorf("every correction of %s lost its input; record them again", agent)
	}
	return out, nil
}

// corpusOf is what a battery is read against: the version it ran. One recorded before
// versions names none and is read against the corpus as it is now, which is all there is,
// and says so by having no pin.
func (s *Server) corpusOf(
	ctx context.Context, agent domain.AgentID, report simulate.Report,
) ([]domain.RegressionCase, error) {
	if report.DatasetVersion == 0 {
		return s.regressions.List(ctx, agent)
	}
	return s.regressions.AtVersion(ctx, agent, report.DatasetVersion)
}

func (s *Server) corpusAt(
	ctx context.Context, agent domain.AgentID, asked int64,
) (int64, []domain.RegressionCase, error) {
	if asked == 0 {
		return s.regressions.Pinned(ctx, agent)
	}
	corpus, err := s.regressions.AtVersion(ctx, agent, asked)
	return asked, corpus, err
}
