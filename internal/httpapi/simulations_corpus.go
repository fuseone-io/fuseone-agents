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
occurrences is what the simulation will replay: an uploaded set, or the corpus.

Replaying the corpus is the battery, and it reads each case back from the
claim check rather than from the run it was corrected from — that copy is the
whole reason the corpus survives retention.
*/
func (s *Server) occurrences(
	ctx context.Context, agent domain.AgentID, simulation string,
	body *openapi.SimulationRequest,
) ([]simulate.Occurrence, error) {
	if body.Corpus != nil && *body.Corpus {
		return s.corpusOccurrences(ctx, agent)
	}

	file := ""
	if body.Cases != nil {
		file = *body.Cases
	}
	cases, err := simulate.Load(ctx, s.cases, simulation, []byte(file))
	if err != nil {
		return nil, err
	}
	out := make([]simulate.Occurrence, 0, len(cases))
	for _, input := range cases {
		out = append(out, simulate.Occurrence{Input: input})
	}
	return out, nil
}

func (s *Server) corpusOccurrences(
	ctx context.Context, agent domain.AgentID,
) ([]simulate.Occurrence, error) {
	if s.regressions == nil {
		return nil, fmt.Errorf("this installation keeps no corrections")
	}
	corpus, err := s.regressions.List(ctx, agent)
	if err != nil {
		return nil, err
	}
	if len(corpus) == 0 {
		return nil, fmt.Errorf("%s has no corrections to re-check", agent)
	}

	out := make([]simulate.Occurrence, 0, len(corpus))
	for _, c := range corpus {
		input, err := s.content.Get(ctx, c.InputRef)
		// Erased is a stated gap, and the report names it. Anything else refuses
		// the whole set: a battery missing a case for no reason reports green on
		// a correction nobody checked.
		if errors.Is(err, domain.ErrContentErased) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("the occurrence of %s could not be read: %w", c.ID, err)
		}
		out = append(out, simulate.Occurrence{ID: c.ID, Input: input})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("every correction of %s lost its input; record them again", agent)
	}
	return out, nil
}
