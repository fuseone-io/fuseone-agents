package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Composition is the graph of who triggers whom, declared here by the
// consumer (PRD SE-10).
type Composition interface {
	Edges(ctx context.Context) ([]domain.EventEdge, error)
}

/*
GetEventGraph is the wiring between agents.

Readable by anybody who can read agents, and deliberately not narrowed by
scope: the point of the graph is that an event crosses areas, and a picture cut
at the reader's own boundary would hide exactly the edges worth reviewing. It
names agents and events, which are configuration, and no run data.
*/
func (s *Server) GetEventGraph(ctx context.Context, _ openapi.GetEventGraphRequestObject) (openapi.GetEventGraphResponseObject, error) {
	if s.composition == nil {
		return openapi.GetEventGraph200JSONResponse{Edges: []openapi.EventEdge{}}, nil
	}

	edges, err := s.composition.Edges(ctx)
	if err != nil {
		return nil, fmt.Errorf("event graph: %w", err)
	}

	items := make([]openapi.EventEdge, 0, len(edges))
	for _, edge := range edges {
		out := openapi.EventEdge{Event: edge.Event}
		if edge.From != "" {
			out.From = ptr(string(edge.From))
		}
		if edge.To != "" {
			out.To = ptr(string(edge.To))
		}
		items = append(items, out)
	}
	return openapi.GetEventGraph200JSONResponse{Edges: items}, nil
}
