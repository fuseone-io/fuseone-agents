package spec_test

import (
	"testing"

	"github.com/fuseone/agents/internal/spec"
)

// A step is an envelope with a gate at its exit: ordered, one at a time, the
// loop free inside it, and advancing only forwards. NT-003 §8 settled the
// shape against the agent that exists.

const withSteps = `---
id: suporte
name: Atendimento
area: cx
provider: openai
model: devstack
tools: [crm.lookup, kb.search, crm.reply]
budget:
  micros: 500000
  steps: 60
steps:
  - name: Identificar o cliente
    reaches: [crm.lookup]
    stops_when: não encontrar o cliente
  - name: Resumir
  - name: Responder
    reaches: [crm.reply]
---
Corpo.
`

func TestParse_stepsNarrowWhatIsReachable(t *testing.T) {
	t.Parallel()

	s, err := spec.Parse("t.md", []byte(withSteps))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(s.Steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(s.Steps))
	}
	// The reply is unreachable until the run is in the step that reaches it,
	// which is strictly tighter than the pack and costs nothing.
	if got := s.Steps[0].Reaches; len(got) != 1 || got[0] != "crm.lookup" {
		t.Errorf("first step reaches %v", got)
	}
}

func TestParse_aStepThatReachesNothing_isStillAStep(t *testing.T) {
	t.Parallel()

	s, _ := spec.Parse("t.md", []byte(withSteps))

	// Summarising calls nothing at all. A model where steps are tool calls
	// cannot represent the simplest real agent in the repository.
	if len(s.Steps) < 2 || s.Steps[1].Name == "" || len(s.Steps[1].Reaches) != 0 {
		t.Errorf("got %+v", s.Steps)
	}
}

func TestParse_noSteps_behavesExactlyAsBefore(t *testing.T) {
	t.Parallel()

	const plain = `---
id: a
name: A
area: cx
provider: openai
model: m
tools: [crm.lookup, crm.reply]
budget:
  micros: 500000
---
Corpo.
`
	s, err := spec.Parse("t.md", []byte(plain))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// One envelope holding the whole pack is today's behaviour, which is what
	// lets steps land without anybody republishing anything.
	env := s.EnvelopeAt(0)
	if len(env) != 2 {
		t.Fatalf("got %v, want the whole pack", env)
	}
}

func TestEnvelopeAt_pastTheLastStep_reachesNothing(t *testing.T) {
	t.Parallel()

	s, _ := spec.Parse("t.md", []byte(withSteps))

	// A run that walked off the end has finished. Falling back to the pack
	// there would make the last step the loosest one in the agent.
	if got := s.EnvelopeAt(9); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

func TestRender_stepsSurviveARoundTrip(t *testing.T) {
	t.Parallel()

	first, err := spec.Parse("t.md", []byte(withSteps))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rendered, err := spec.Render(first)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	again, err := spec.Parse("t.md", rendered)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}

	// The version is the digest of the bytes, so a definition that did not
	// survive rendering would publish as a different agent than it reads as.
	if len(again.Steps) != len(first.Steps) || again.Steps[0].StopsWhen != first.Steps[0].StopsWhen {
		t.Errorf("got %+v, want %+v", again.Steps, first.Steps)
	}
}
