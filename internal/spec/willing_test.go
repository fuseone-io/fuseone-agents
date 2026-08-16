package spec_test

import (
	"context"
	"strings"
	"testing"
)

/*
Whether a message may start an agent, asked of the version that would run.

The consumer needs this before it opens anything, and it is a property of the
published text rather than of the agent's name: an author who added the trigger
this morning did not make yesterday's pinned version startable by text, and one
who removed it did not make today's unstartable.
*/

const byConversation = `---
id: helper
name: Helper
area: cx
provider: openai
model: test-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
triggers:
  - { type: channel }
---

Answer what you are asked.
`

func TestStartableFromConversation_theVersionThatDeclaredIt_isWilling(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	s := published(t, byConversation)
	if err := r.Publish(ctx, s, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	willing, err := r.StartableFromConversation(ctx, "helper", s.Version)
	if err != nil {
		t.Fatalf("StartableFromConversation: %v", err)
	}
	if !willing {
		t.Error("not willing, but the published version declares the trigger")
	}
}

// An agent that never declared it cannot be started by any message, however
// the conversations are mapped. That is the half of the rule worth being able
// to state: "this one is internal, never startable by text".
func TestStartableFromConversation_anAgentThatNeverDeclaredIt_isNot(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	s := published(t, definition)
	if err := r.Publish(ctx, s, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	willing, err := r.StartableFromConversation(ctx, "triage", s.Version)
	if err != nil {
		t.Fatalf("StartableFromConversation: %v", err)
	}
	if willing {
		t.Error("willing, but the published version declares only a schedule")
	}
}

/*
A version nobody published is an error, not an unwilling agent.

The two are answered differently and must be: unwilling closes the ask and
tells the person their agent cannot be started that way, which is a sentence
about their agent. A read that failed is a sentence about us, and answering it
with the first one sends somebody to add a trigger to a spec that already has
one.
*/
func TestStartableFromConversation_aVersionNobodyPublished_isAnErrorNotARefusal(t *testing.T) {
	r := openRegistry(t)

	_, err := r.StartableFromConversation(context.Background(), "helper", "v-never")
	if err == nil {
		t.Fatal("no error; a read that failed would be answered as a refusal")
	}
	if !strings.Contains(err.Error(), "helper") {
		t.Errorf("err = %v, want the agent it could not read", err)
	}
}
