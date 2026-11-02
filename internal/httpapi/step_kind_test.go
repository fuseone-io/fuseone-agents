package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Every kind the ledger holds is a value of the API's enum. A kind missing
// there is a step the trail serves outside its own schema, and a console
// generated from that schema does not know how to draw it.
func TestStepKind_everyLedgerKind_isInTheAPIEnum(t *testing.T) {
	t.Parallel()

	for _, kind := range domain.StepKinds() {
		if !openapi.StepKind(kind).Valid() {
			t.Errorf("%s: not in the StepKind enum", kind)
		}
	}
	if openapi.StepKindWorkItemLinked != openapi.StepKind(domain.StepWorkItemLinked) {
		t.Fatalf("enum value %q, ledger kind %q", openapi.StepKindWorkItemLinked, domain.StepWorkItemLinked)
	}
}

// A link served on a run's trail decodes into the schema the API documents
// for it, field for field: the payload is the ledger's own JSON, so a schema
// that named a field differently would describe a step nobody receives.
func TestToStep_workItemLinked_servesThePayloadTheSchemaDocuments(t *testing.T) {
	t.Parallel()

	sealed := domain.WorkItemLinkedPayload{
		Alias: "github:fuseone-io/fuseone-agents#240", Root: "github:fuseone-io/fuseone-agents#204",
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: 12},
	}
	raw, err := json.Marshal(sealed)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	served := toStep(domain.Step{Seq: 13, Kind: domain.StepWorkItemLinked, Payload: raw})
	if served.Kind != openapi.StepKindWorkItemLinked || served.Payload == nil {
		t.Fatalf("served = %+v, want the link with its payload", served)
	}

	body, err := json.Marshal(*served.Payload)
	if err != nil {
		t.Fatalf("Marshal served payload: %v", err)
	}
	var got openapi.WorkItemLinkedPayload
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode into the documented schema: %v", err)
	}
	want := openapi.WorkItemLinkedPayload{
		Alias: string(sealed.Alias), Root: string(sealed.Root),
		Proof: openapi.WorkItemLinkProof{Kind: openapi.WorkItemLinkProofKindToolReturned, Seq: 12},
	}
	if got != want {
		t.Fatalf("served payload = %+v, want %+v", got, want)
	}
	if !got.Proof.Kind.Valid() || !openapi.WorkItemLinkProofKindApprovalDecided.Valid() {
		t.Fatalf("proof kind %q is outside the documented enum", got.Proof.Kind)
	}
}
