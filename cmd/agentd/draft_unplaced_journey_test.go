package main

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
A run of an agent with steps is offered only what some step reaches, so a tool
granted and placed in no step is one the run reports as not available while the
list of tools shows it granted. The workbench says so: the draft lists it, and
publishing from the workbench is refused until it is placed in a step or taken
out of the tools.
*/
func TestDraftPublish_aToolNoStepReaches_isListedAndRefusedUntilPlaced(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.ready(t)
	tag = w.accept(t, tag, draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})
	for _, op := range []string{
		`{"kind":"add_tool","tool":"kb.search"}`,
		`{"kind":"add_step","step":{"name":"Consultar","reaches":["crm.lookup"]}}`,
	} {
		rec := w.apply(t, areaAuthor("factory"), w.agent, tag, op)
		wantStatus(t, op, rec, http.StatusOK)
		tag = rec.Header().Get("ETag")
	}

	var read openapi.AgentDraft
	decode(t, w.read(t, areaAuthor("factory"), w.agent), &read)
	if !slices.Equal(read.Unplaced, []string{"kb.search"}) {
		t.Fatalf("unplaced = %v, want [kb.search]", read.Unplaced)
	}
	refused := w.publish(t, areaAuthor("factory"), tag)
	wantStatus(t, "an unplaced tool", refused, http.StatusBadRequest)
	if problemType(t, refused) != "fuseone:publish-unplaced-tools" || !strings.Contains(refused.Body.String(), "kb.search") {
		t.Fatalf("refusal = %s, want publish-unplaced-tools naming kb.search", refused.Body.String())
	}

	placed := w.apply(t, areaAuthor("factory"), w.agent, tag,
		`{"kind":"edit_step","stepName":"Consultar","step":{"name":"Consultar","reaches":["crm.lookup","kb.search"]}}`)
	wantStatus(t, "place it", placed, http.StatusOK)
	wantStatus(t, "publish once placed", w.publish(t, areaAuthor("factory"), placed.Header().Get("ETag")), http.StatusOK)
}
