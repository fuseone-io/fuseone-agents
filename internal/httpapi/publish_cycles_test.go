package httpapi

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// emitsAndListens is a definition that publishes one event and starts on
// another.
func emitsAndListens(emits, listens string) *openapi.PublishAgentJSONRequestBody {
	return definition(func(d *openapi.AgentDefinition) {
		d.Emits = &[]openapi.AgentEvent{{Event: emits}}
		d.Triggers = &[]openapi.AgentTrigger{{Type: "event", Event: ptr(listens)}}
	})
}

// looping is a current version in acme/cx that publishes one event and
// starts on another, as the fake publisher holds it.
func looping(id domain.AgentID, emits, listens string) spec.Spec {
	return loopingIn("cx", id, emits, listens)
}

// loopingIn is looping in another area of acme.
func loopingIn(area domain.AreaID, id domain.AgentID, emits, listens string) spec.Spec {
	return spec.Spec{
		ID: id, Company: "acme", Area: area, Emits: spec.Emits{{Event: emits}},
		Triggers: []spec.Trigger{{Type: spec.TriggerEvent, Event: listens}},
	}
}

func publishAs(t *testing.T, pub *publisher, agent string, body *openapi.PublishAgentJSONRequestBody) openapi.AgentPublished {
	t.Helper()
	return publishBy(t, inArea("cx", domain.RoleAuthor), pub, agent, body)
}

// publishBy is publishAs for a caller of the test's choosing.
func publishBy(
	t *testing.T, caller context.Context, pub *publisher, agent string, body *openapi.PublishAgentJSONRequestBody,
) openapi.AgentPublished {
	t.Helper()
	got, err := tryPublishBy(caller, publishServer(t, pub), agent, body)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// tryPublish is publishAs for a goroutine, which may not stop the test.
func tryPublish(s *Server, agent string, body *openapi.PublishAgentJSONRequestBody) (openapi.AgentPublished, error) {
	return tryPublishBy(inArea("cx", domain.RoleAuthor), s, agent, body)
}

func tryPublishBy(
	caller context.Context, s *Server, agent string, body *openapi.PublishAgentJSONRequestBody,
) (openapi.AgentPublished, error) {
	resp, err := s.PublishAgent(caller,
		openapi.PublishAgentRequestObject{AgentId: agent, Body: body})
	if err != nil {
		return openapi.AgentPublished{}, fmt.Errorf("PublishAgent %s: %w", agent, err)
	}
	ok, is := resp.(openapi.PublishAgent200JSONResponse)
	if !is {
		return openapi.AgentPublished{}, fmt.Errorf("PublishAgent %s = %#v, want 200", agent, resp)
	}
	return openapi.AgentPublished(ok), nil
}

// The Developer ⇄ Reviewer loop is a cycle on purpose: the version is
// written, and the answer names both agents and the hop limit that bounds it.
func TestPublishAgent_closesAnEventCycle_publishesAndWarnsNamingEveryAgent(t *testing.T) {
	t.Parallel()
	pub := newPublisher(looping("reviewer", "review.done", "code.ready"))

	got := publishAs(t, pub, "developer", emitsAndListens("code.ready", "review.done"))

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
	pub := newPublisher(
		looping("reviewer", "review.done", "code.ready"),
		looping("developer", "code.ready", "review.done"),
	)

	got := publishAs(t, pub, "developer", emitsAndListens("code.ready", "ticket.new"))

	if got.EventCycleWarning != nil {
		t.Errorf("warning = %+v, want none", got.EventCycleWarning)
	}
}

/*
A graph too large to search within the bound is said to be unchecked, not
passed over in silence.

Silence is what a publish that closes no cycle answers, so a search that ran
out before it could tell would read as "no cycle" — the one answer it has no
grounds for.
*/
func TestPublishAgent_aGraphBeyondTheWorkBound_saysItWasNotChecked(t *testing.T) {
	t.Parallel()
	noisy := spec.Spec{ID: "noisy"}
	for i := range domain.MaxCycleWork {
		noisy.Triggers = append(noisy.Triggers,
			spec.Trigger{Type: spec.TriggerEvent, Event: fmt.Sprintf("e%06d", i)})
	}

	got := publishAs(t, newPublisher(noisy), "developer", emitsAndListens("code.ready", "review.done"))

	want := &openapi.EventCycleWarning{Cycles: [][]string{}, Truncated: true, HopLimit: domain.MaxEventHops}
	if !reflect.DeepEqual(got.EventCycleWarning, want) {
		t.Errorf("warning = %+v, want %+v", got.EventCycleWarning, want)
	}
}

// More cycles than one answer names: the answer says the list stops short,
// so nobody reads sixteen loops as all of them.
func TestPublishAgent_closesMoreCyclesThanTheAnswerNames_saysItIsTruncated(t *testing.T) {
	t.Parallel()
	var reviewers []spec.Spec
	for i := range domain.MaxEventCycles + 4 {
		reviewers = append(reviewers,
			looping(domain.AgentID(fmt.Sprintf("reviewer-%02d", i)), "review.done", "code.ready"))
	}

	got := publishAs(t, newPublisher(reviewers...), "developer", emitsAndListens("code.ready", "review.done"))

	if got.EventCycleWarning == nil || !got.EventCycleWarning.Truncated ||
		len(got.EventCycleWarning.Cycles) != domain.MaxEventCycles {
		t.Errorf("warning = %+v, want %d cycles, said to be truncated", got.EventCycleWarning, domain.MaxEventCycles)
	}
}

/*
The fake serialises a judgement with the write after it, as spec.Publisher
does under the event graph's lock.

A fake that judged and wrote without it would pass every test above and
still let two concurrent publications, each closing half of a loop, both miss
it — the one thing the real publisher was changed to prevent.
*/
func TestPublishAgent_twoPublicationsCloseACycleTogether_theSecondToJudgeWarns(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	judged, release := make(chan struct{}), make(chan struct{})
	pub.afterJudging = func(agent domain.AgentID) {
		if agent == "developer" {
			close(judged)
			<-release
		}
	}
	server := publishServer(t, pub)
	first, second := make(chan openapi.AgentPublished, 1), make(chan openapi.AgentPublished, 1)
	go func() {
		got, _ := tryPublish(server, "developer", emitsAndListens("code.ready", "review.done"))
		first <- got
	}()
	<-judged

	go func() {
		got, _ := tryPublish(server, "reviewer", emitsAndListens("review.done", "code.ready"))
		second <- got
	}()
	for pub.waiting() == 0 {
		select {
		case <-second:
			t.Fatal("reviewer published on the graph from before developer's write")
		case <-time.After(time.Millisecond):
		}
	}
	close(release)

	if got := <-first; got.EventCycleWarning != nil {
		t.Errorf("developer warned %+v; it judged before reviewer existed", got.EventCycleWarning)
	}
	if got := <-second; got.EventCycleWarning == nil {
		t.Error("reviewer was not warned of the cycle it closed")
	}
}

/*
A cycle through an agent the caller may not read is named without that
agent (review round 2).

The cycle is judged across the whole installation, because it is real
wherever its agents live. But an author with the right to publish in cx is
not owed the names of agents in billing, which GetAgent and ListAgents hide
from them: the answer keeps every step of the loop and the hop limit, and
writes an agent outside the caller's read access as an empty name.
*/
func TestPublishAgent_aCycleThroughAnAreaTheCallerCannotRead_hidesThoseAgents(t *testing.T) {
	t.Parallel()
	pub := newPublisher(loopingIn("billing", "invoicer", "review.done", "code.ready"))

	got := publishAs(t, pub, "developer", emitsAndListens("code.ready", "review.done"))

	want := &openapi.EventCycleWarning{Cycles: [][]string{{"developer", ""}}, HopLimit: domain.MaxEventHops}
	if !reflect.DeepEqual(got.EventCycleWarning, want) {
		t.Errorf("warning = %+v, want %+v", got.EventCycleWarning, want)
	}
}

// A caller who may read agents everywhere is told every name.
func TestPublishAgent_aCycleAcrossAreas_isNamedInFullToAnInstallationWideReader(t *testing.T) {
	t.Parallel()
	pub := newPublisher(loopingIn("billing", "invoicer", "review.done", "code.ready"))

	got := publishBy(t, everywhere(domain.RoleAdmin), pub, "developer", emitsAndListens("code.ready", "review.done"))

	want := &openapi.EventCycleWarning{Cycles: [][]string{{"developer", "invoicer"}}, HopLimit: domain.MaxEventHops}
	if !reflect.DeepEqual(got.EventCycleWarning, want) {
		t.Errorf("warning = %+v, want %+v", got.EventCycleWarning, want)
	}
}
