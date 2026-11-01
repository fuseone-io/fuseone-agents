package httpapi

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// graph is the published event graph, as the registry draws it.
type graph struct {
	edges []domain.EventEdge
	err   error
}

func (g graph) Edges(context.Context) ([]domain.EventEdge, error) { return g.edges, g.err }

// emitsAndListens is a definition that publishes one event and starts on
// another.
func emitsAndListens(emits, listens string) *openapi.PublishAgentJSONRequestBody {
	return definition(func(d *openapi.AgentDefinition) {
		d.Emits = &[]openapi.AgentEvent{{Event: emits}}
		d.Triggers = &[]openapi.AgentTrigger{{Type: "event", Event: ptr(listens)}}
	})
}

func publishWithGraph(t *testing.T, g graph, agent string, body *openapi.PublishAgentJSONRequestBody) (openapi.AgentPublished, *publisher) {
	t.Helper()
	pub := newPublisher()
	resp, err := publishServer(t, pub).WithComposition(g).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: agent, Body: body},
	)
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	ok, is := resp.(openapi.PublishAgent200JSONResponse)
	if !is {
		t.Fatalf("response = %#v, want 200", resp)
	}
	return openapi.AgentPublished(ok), pub
}

// The Developer ⇄ Reviewer loop is a cycle on purpose: the version is
// written, and the answer names both agents and the hop limit that bounds it.
func TestPublishAgent_closesAnEventCycle_publishesAndWarnsNamingEveryAgent(t *testing.T) {
	t.Parallel()
	reviewer := graph{edges: []domain.EventEdge{
		{From: "reviewer", Event: "review.done", To: "developer"},
		{From: "developer", Event: "code.ready", To: "reviewer"},
	}}

	got, pub := publishWithGraph(t, reviewer, "developer", emitsAndListens("code.ready", "review.done"))

	want := &openapi.EventCycleWarning{Cycles: [][]string{{"developer", "reviewer"}}, HopLimit: domain.MaxEventHops}
	if !reflect.DeepEqual(got.EventCycleWarning, want) {
		t.Errorf("warning = %+v, want %+v", got.EventCycleWarning, want)
	}
	if len(pub.published) != 1 {
		t.Errorf("published %d versions, want the one: a cycle is a warning, not a refusal", len(pub.published))
	}
}

// What the version declares is judged, not what its agent's current one did:
// a version that stops listening leaves the cycle it used to close.
func TestPublishAgent_stopsListeningToTheCycle_answersWithNoWarning(t *testing.T) {
	t.Parallel()
	looped := graph{edges: []domain.EventEdge{
		{From: "reviewer", Event: "review.done", To: "developer"},
		{From: "developer", Event: "code.ready", To: "reviewer"},
	}}

	got, _ := publishWithGraph(t, looped, "developer", emitsAndListens("code.ready", "ticket.new"))

	if got.EventCycleWarning != nil {
		t.Errorf("warning = %+v, want none", got.EventCycleWarning)
	}
}

// A graph that cannot be read refuses before anything is written, rather
// than publishing and then answering an error for a version that exists.
func TestPublishAgent_theGraphCannotBeRead_writesNothing(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	_, err := publishServer(t, pub).WithComposition(graph{err: errors.New("down")}).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "developer", Body: emitsAndListens("a", "b")},
	)

	if err == nil || len(pub.published) != 0 {
		t.Errorf("err = %v, published = %d; want an error and nothing written", err, len(pub.published))
	}
}

// More cycles than one answer names: the answer says the list stops short,
// so nobody reads sixteen loops as all of them.
func TestPublishAgent_closesMoreCyclesThanTheAnswerNames_saysItIsTruncated(t *testing.T) {
	t.Parallel()
	var many graph
	for i := range domain.MaxEventCycles + 4 {
		reviewer := domain.AgentID(fmt.Sprintf("reviewer-%02d", i))
		many.edges = append(many.edges,
			domain.EventEdge{From: "developer", Event: "code.ready", To: reviewer},
			domain.EventEdge{From: reviewer, Event: "review.done", To: "developer"})
	}

	got, _ := publishWithGraph(t, many, "developer", emitsAndListens("code.ready", "review.done"))

	if got.EventCycleWarning == nil || !got.EventCycleWarning.Truncated ||
		len(got.EventCycleWarning.Cycles) != domain.MaxEventCycles {
		t.Errorf("warning = %+v, want %d cycles, said to be truncated", got.EventCycleWarning, domain.MaxEventCycles)
	}
}
