package e2e_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
Every step says who acted, as whom, under which version (PRD AU-03, AU-05).

The agent is a principal in its own right and the trail always records the
pair: agent X acted on behalf of Y. Sealing a step enforces the run and the
scope and nothing else, so a step kind that forgot the rest would be written
happily, and the hole would only show up in an export somebody reads years
later.

Three step kinds were added to this platform in a single evening. This is the
assertion that makes the fourth one fail loudly instead of quietly.
*/
func TestTrail_everyStepSaysWhoActedAndAsWhom(t *testing.T) {
	eachLedger(t, "no step is anonymous", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenStall)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite, CompensatedBy: "crm.unnote",
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		// A run taken through every ending this platform has, so the newest
		// step kinds are in the trail too and not just the oldest.
		p.open(t, "run-identity-1")
		p.settle(t, "run-identity-1")
		p.resume(t, "run-identity-1")
		p.settle(t, "run-identity-1")
		p.abandon(t, "run-identity-1")
		p.settle(t, "run-identity-1")

		steps := p.steps(t, "run-identity-1")
		if len(steps) < 10 {
			t.Fatalf("the run produced %d steps, too few to be a fair sample", len(steps))
		}

		kinds := map[domain.StepKind]bool{}
		for _, step := range steps {
			kinds[step.Kind] = true
			switch {
			case step.Scope.Company == "" || step.Scope.Area == "":
				t.Errorf("step %d (%s) has no scope", step.Seq, step.Kind)
			case step.AgentID == "":
				t.Errorf("step %d (%s) does not say which agent acted", step.Seq, step.Kind)
			case step.VersionID == "":
				t.Errorf("step %d (%s) does not say which version acted", step.Seq, step.Kind)
			case step.OnBehalfOf == "":
				t.Errorf("step %d (%s) does not say who it acted for", step.Seq, step.Kind)
			}
		}

		// The endings are the point: a sample that never reached them would
		// certify the step kinds that were always fine.
		for _, kind := range []domain.StepKind{
			domain.StepParked, domain.StepResumed,
			domain.StepCompensated, domain.StepAbandoned, domain.StepFailed,
		} {
			if !kinds[kind] {
				t.Errorf("the run never produced a %s step; the sample proves less than it looks", kind)
			}
		}
	})
}
