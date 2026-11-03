package domain_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// An event-opened run inherits its source's own claim before anything the
// source itself inherited: the run that did the work on a card is a better
// witness of it than the run that only came from one.
func TestInheritWorkItem_sourceClaimed_claimWinsOverItsInheritance(t *testing.T) {
	t.Parallel()
	inherited := &domain.RunWorkItem{Ref: "github:o/r#1", Source: domain.WorkItemFromGitHubDelivery}

	got := domain.InheritWorkItem("run-dev", "github:o/r#2", inherited)

	want := &domain.RunWorkItem{Ref: "github:o/r#2", Source: domain.WorkItemFromEvent, FromRun: "run-dev"}
	if got == nil || *got != *want {
		t.Fatalf("InheritWorkItem = %+v, want %+v", got, want)
	}
}

func TestInheritWorkItem_sourceWithoutClaim_passesItsInheritanceFromItself(t *testing.T) {
	t.Parallel()
	inherited := &domain.RunWorkItem{Ref: "github:o/r#1", Source: domain.WorkItemFromGitHubDelivery}

	got := domain.InheritWorkItem("run-triage", "", inherited)

	// The source of the passed-on attribution is the event, and it names the
	// run it came from — never the delivery two hops back.
	want := &domain.RunWorkItem{Ref: "github:o/r#1", Source: domain.WorkItemFromEvent, FromRun: "run-triage"}
	if got == nil || *got != *want {
		t.Fatalf("InheritWorkItem = %+v, want %+v", got, want)
	}
}

func TestInheritWorkItem_neither_inheritsNothing(t *testing.T) {
	t.Parallel()
	if got := domain.InheritWorkItem("run-x", "", nil); got != nil {
		t.Fatalf("InheritWorkItem = %+v, want nil", got)
	}
}

// A malformed inheritance on the source is not passed on: the opener would
// refuse the run it was sealed on, and the listener must still open.
func TestInheritWorkItem_invalidInheritance_inheritsNothing(t *testing.T) {
	t.Parallel()
	bad := &domain.RunWorkItem{Ref: "GITHUB:o/r#1", Source: domain.WorkItemFromEvent, FromRun: "run-0"}
	if got := domain.InheritWorkItem("run-x", "", bad); got != nil {
		t.Fatalf("InheritWorkItem = %+v, want nil", got)
	}
}

func TestRunWorkItemValidate(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		item domain.RunWorkItem
		ok   bool
	}{
		"event":                {domain.RunWorkItem{Ref: "github:o/r#1", Source: "event", FromRun: "run-1"}, true},
		"event, board item":    {domain.RunWorkItem{Ref: "ghproject:PVTI_x", Source: "event", FromRun: "run-1"}, true},
		"delivery":             {domain.RunWorkItem{Ref: "github:o/r#1", Source: "github_delivery"}, true},
		"empty ref":            {domain.RunWorkItem{Source: "event", FromRun: "run-1"}, false},
		"non-canonical ref":    {domain.RunWorkItem{Ref: "github:O/r#1", Source: "event", FromRun: "run-1"}, false},
		"padded ref":           {domain.RunWorkItem{Ref: " github:o/r#1", Source: "event", FromRun: "run-1"}, false},
		"unknown source":       {domain.RunWorkItem{Ref: "github:o/r#1", Source: "claim", FromRun: "run-1"}, false},
		"event without run":    {domain.RunWorkItem{Ref: "github:o/r#1", Source: "event"}, false},
		"delivery with run":    {domain.RunWorkItem{Ref: "github:o/r#1", Source: "github_delivery", FromRun: "run-1"}, false},
		"delivery, board item": {domain.RunWorkItem{Ref: "ghproject:PVTI_x", Source: "github_delivery"}, false},
	}
	for name, c := range cases {
		err := c.item.Validate()
		if c.ok && err != nil {
			t.Errorf("%s: %v, want valid", name, err)
		}
		if !c.ok && !errors.Is(err, domain.ErrRunWorkItemInvalid) {
			t.Errorf("%s: %v, want ErrRunWorkItemInvalid", name, err)
		}
	}
}

// The field's wire shape is fixed: it is sealed on run_started for good.
func TestRunStartedPayload_workItem_wireShape(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(domain.RunStartedPayload{Trigger: "event", WorkItem: &domain.RunWorkItem{
		Ref: "github:o/r#1", Source: domain.WorkItemFromEvent, FromRun: "run-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"trigger":"event","work_item":{"ref":"github:o/r#1","source":"event","from_run":"run-1"}}`
	if string(raw) != want {
		t.Fatalf("wire = %s, want %s", raw, want)
	}
}

// Rolling upgrade: a start an older opener sealed has no work item and
// decodes as one with none; a start a newer one sealed decodes under the
// shape an older reader knows, its extra field ignored.
func TestRunStartedPayload_oldAndNewShapes_decode(t *testing.T) {
	t.Parallel()
	var old domain.RunStartedPayload
	if err := json.Unmarshal([]byte(`{"trigger":"event","chain":{"root":"run-0","hop":1}}`), &old); err != nil {
		t.Fatalf("old shape: %v", err)
	}
	if old.WorkItem != nil || old.Chain == nil {
		t.Fatalf("old shape decoded as %+v", old)
	}
	var older struct {
		Trigger string             `json:"trigger"`
		Chain   *domain.EventChain `json:"chain,omitempty"`
	}
	newer := `{"trigger":"event","chain":{"root":"run-0","hop":1},` +
		`"work_item":{"ref":"github:o/r#1","source":"event","from_run":"run-0"}}`
	if err := json.Unmarshal([]byte(newer), &older); err != nil || older.Chain == nil {
		t.Fatalf("an older reader of the new shape: %+v, %v", older, err)
	}
}
