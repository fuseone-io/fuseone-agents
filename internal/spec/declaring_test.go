package spec_test

import (
	"context"
	"testing"
)

/*
Who would notice a tool being taken away.

Removing a tool from a server's surface is a decision about scope, and taking
it out from under an agent that names it is a consequence of that decision
rather than part of it. The run stops at the Gate with an unknown capability,
which is correct and is also the worst place to find out.

So the question is answerable before the change: of the agents this
installation runs, which ones declare this tool.
*/

const declaringLookup = `---
id: triagem
name: Triagem
area: cx
provider: openai
model: test-model
tools:
  - crm.lookup
  - crm.note
budget:
  micros: 500000
  steps: 60
triggers:
  - { type: cron, schedule: "*/15 * * * *" }
---

Read the ticket.
`

const declaringNothingOfTheSort = `---
id: cobranca
name: Cobrança
area: cx
provider: openai
model: test-model
tools:
  - erp.invoice
budget:
  micros: 500000
  steps: 60
triggers:
  - { type: cron, schedule: "*/15 * * * *" }
---

Chase the invoice.
`

func TestDeclaring_answersWhichAgentsNameATool(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	for _, source := range []string{declaringLookup, declaringNothingOfTheSort} {
		if err := r.Publish(ctx, published(t, source), "usr_ana", "acme"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	found, err := r.Declaring(ctx, "crm.lookup")
	if err != nil {
		t.Fatalf("Declaring: %v", err)
	}
	if len(found) != 1 || found[0].ID != "triagem" {
		t.Fatalf("found = %+v, want the one agent that names it", found)
	}
}

// A tool nobody names is nobody's problem, and says so rather than answering
// with everything or with an error.
func TestDeclaring_aToolNoAgentNames_isEmptyAndNotAnError(t *testing.T) {
	r := openRegistry(t)
	if err := r.Publish(context.Background(), published(t, declaringLookup),
		"usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	found, err := r.Declaring(context.Background(), "crm.unused")
	if err != nil {
		t.Fatalf("Declaring: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("found = %+v, want nobody", found)
	}
}

/*
Asked of the version that would run, not of every version ever published.

An old version that names the tool is pinned to runs already recorded and
cannot be started again, so counting it would tell somebody they are about to
break an agent they replaced last month.
*/
func TestDeclaring_aVersionThatIsNoLongerCurrent_isNotCounted(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()

	if err := r.Publish(ctx, published(t, declaringLookup), "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// The same agent, republished without the tool.
	withoutIt := published(t, `---
id: triagem
name: Triagem
area: cx
provider: openai
model: test-model
tools:
  - crm.note
budget:
  micros: 500000
  steps: 60
triggers:
  - { type: cron, schedule: "*/15 * * * *" }
---

Read the ticket, differently.
`)
	if err := r.Publish(ctx, withoutIt, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish again: %v", err)
	}
	if err := r.MakeCurrent(ctx, "triagem", withoutIt.Version); err != nil {
		t.Fatalf("MakeCurrent: %v", err)
	}

	found, err := r.Declaring(ctx, "crm.lookup")
	if err != nil {
		t.Fatalf("Declaring: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("found = %+v; the warning names an agent that no longer uses it", found)
	}
}
