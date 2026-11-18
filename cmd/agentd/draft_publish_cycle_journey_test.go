package main

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Publishing from the workbench says what publishing from the form said: the
version that closes an event cycle is written, and the answer names the cycle
(#227). The workbench is now the only door the console has, so a route that
dropped the warning would publish every unintended loop in silence.

Through the router, the JSON on the wire and the Postgres publisher, as the
form's journey is: the graph is judged inside the publication's transaction,
and only the real publisher does that.
*/
func TestDraftPublish_theVersionThatClosesACycle_publishesAndNamesIt(t *testing.T) {
	w := newPublishWorld(t)
	other := w.agent + "-reviewer"
	ready, done := w.agent+".code.ready", w.agent+".review.done"
	publishRoute{handler: w.handler}.publishIn(t, openPrincipal(), "factory", other, done, ready)

	tag := w.accept(t, w.ready(t), draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})
	for _, op := range []string{
		`{"kind":"set_emits","emits":[{"event":"` + ready + `"}]}`,
		`{"kind":"set_triggers","triggers":[{"type":"event","event":"` + done + `"}]}`,
	} {
		rec := w.apply(t, areaAuthor("factory"), w.agent, tag, op)
		wantStatus(t, op, rec, http.StatusOK)
		tag = rec.Header().Get("ETag")
	}

	rec := w.publish(t, areaAuthor("factory"), tag)
	wantStatus(t, "publish", rec, http.StatusOK)
	var resp openapi.PublishAgentDraft200JSONResponse
	decode(t, rec, &resp.Body)
	want := &openapi.EventCycleWarning{Cycles: [][]string{{w.agent, other}}, HopLimit: 8}
	if !reflect.DeepEqual(resp.Body.EventCycleWarning, want) {
		t.Fatalf("warning = %+v, want %+v", resp.Body.EventCycleWarning, want)
	}
	if !resp.Body.Created {
		t.Error("the version closing the cycle was not written; a cycle is a warning, not a refusal")
	}
}
