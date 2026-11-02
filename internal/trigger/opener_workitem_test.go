package trigger_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

// An inherited work item (#204, slice 3) is sealed on run_started exactly as
// the platform handed it to the opener.
func TestOpen_workItemProjected(t *testing.T) {
	t.Parallel()
	opener, store := openerFor(t)
	item := &domain.RunWorkItem{Ref: "github:o/r#7", Source: domain.WorkItemFromGitHubDelivery}

	got, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "gh:delivery-1", Trigger: "webhook", WorkItem: item,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	steps, _ := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	var payload domain.RunStartedPayload
	decode(t, steps[0].Payload, &payload)
	if payload.WorkItem == nil || *payload.WorkItem != *item {
		t.Fatalf("sealed work item = %+v, want %+v", payload.WorkItem, item)
	}
	// Attribution only: the run holds no card, so a claim is still its own.
	trail := trailOf(t, store, "github:o/r#7")
	if len(trail.Runs) != 1 || trail.Runs[0].RunID != got.RunID ||
		trail.Runs[0].Link != domain.WorkItemLinkGitHubDelivery {
		t.Fatalf("trail = %+v, want the run by its delivery", trail.Runs)
	}
}

// An inheritance that does not validate is refused before anything exists:
// no run, no input stored.
func TestOpen_invalidWorkItem_refused(t *testing.T) {
	t.Parallel()
	for name, item := range map[string]*domain.RunWorkItem{
		"non-canonical":       {Ref: "github:O/R#7", Source: domain.WorkItemFromGitHubDelivery},
		"unknown source":      {Ref: "github:o/r#7", Source: "claim"},
		"event without a run": {Ref: "github:o/r#7", Source: domain.WorkItemFromEvent},
	} {
		opener, store := openerFor(t)
		_, err := opener.Open(t.Context(), trigger.Request{
			Agent: "triage", IdemKey: "k-" + name, Input: []byte("x"), WorkItem: item,
		})
		if !errors.Is(err, domain.ErrRunWorkItemInvalid) {
			t.Errorf("%s: err = %v, want ErrRunWorkItemInvalid", name, err)
		}
		if runs, _ := store.Runs(context.Background()); len(runs) != 0 {
			t.Errorf("%s: %d runs opened", name, len(runs))
		}
	}
}

// What a run is about — a body, an ask, an event's context — never names its
// work item, however much it looks like one: only the platform's own field
// does.
func TestOpen_workItemFromModelInput_ignored(t *testing.T) {
	t.Parallel()
	opener, store := openerFor(t)
	input := `{"work_item":{"ref":"github:o/r#9","source":"github_delivery"},` +
		`"workItem":"github:o/r#9","ref":"github:o/r#9","issue":{"number":9},` +
		`"repository":{"full_name":"o/r"}}`

	got, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "manual-1", Trigger: "manual", Input: []byte(input),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	steps, _ := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if strings.Contains(string(steps[0].Payload), "work_item") {
		t.Fatalf("run_started = %s; the input named the work item", steps[0].Payload)
	}
	if trail := trailOf(t, store, "github:o/r#9"); len(trail.Runs) != 0 {
		t.Fatalf("trail = %+v; the input put the run on a card", trail.Runs)
	}
}

func trailOf(t *testing.T, store interface {
	WorkItemTrail(context.Context, domain.RunFilter, *domain.RunFilter, domain.WorkItemRef,
		domain.WorkItemPage) (domain.WorkItemTrail, error)
}, ref domain.WorkItemRef,
) domain.WorkItemTrail {
	t.Helper()
	trail, err := store.WorkItemTrail(context.Background(), domain.RunFilter{}, nil, ref, domain.WorkItemPage{})
	if err != nil {
		t.Fatalf("WorkItemTrail: %v", err)
	}
	return trail
}
