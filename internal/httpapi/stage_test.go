package httpapi

import (
	gocontext "context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Promotion is the moment an agent starts doing things nobody will be asked
// about, so what matters is what it takes to get there and what never blocks
// coming back.

type fakeStages struct {
	stage domain.Stage
	set   domain.Stage
}

func (f *fakeStages) StageOf(gocontext.Context, domain.AgentID) (domain.Stage, error) {
	return f.stage, nil
}

func (f *fakeStages) Stages(gocontext.Context) (map[domain.AgentID]domain.Stage, error) {
	return map[domain.AgentID]domain.Stage{"triage": f.stage}, nil
}

func (f *fakeStages) SetStage(_ gocontext.Context, _ domain.AgentID, s domain.Stage, _ domain.UserID) error {
	f.set = s
	return nil
}

func staging(t *testing.T, stages *fakeStages, store *ledger.Memory) *Server {
	t.Helper()
	return NewServer(store, "test").WithAgents(triggerable(t)).WithPromotions(stages)
}

func toStage(stage openapi.Stage) openapi.SetAgentStageRequestObject {
	return openapi.SetAgentStageRequestObject{
		AgentId: "triage",
		Body:    &openapi.SetAgentStageJSONRequestBody{Stage: stage},
	}
}

func TestSetAgentStage_aDraftNobodySimulated_cannotBePromoted(t *testing.T) {
	t.Parallel()

	stages := &fakeStages{stage: domain.StageDraft}
	resp, err := staging(t, stages, ledger.NewMemory()).
		SetAgentStage(inArea("cx", domain.RoleAuthor), toStage("copilot"))
	if err != nil {
		t.Fatalf("SetAgentStage: %v", err)
	}

	// The only check that exists before an agent touches real work. A
	// promotion that skipped it would make the whole authoring path optional.
	if _, ok := resp.(openapi.SetAgentStage400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if stages.set != "" {
		t.Error("it was promoted anyway")
	}
}

func TestSetAgentStage_aDemotion_isNeverRefused(t *testing.T) {
	t.Parallel()

	// The platform demotes on its own when people keep overruling an agent. A
	// person must never have less power than the sweep.
	stages := &fakeStages{stage: domain.StageAutonomous}
	resp, err := staging(t, stages, ledger.NewMemory()).
		SetAgentStage(inArea("cx", domain.RoleAuthor), toStage("draft"))
	if err != nil {
		t.Fatalf("SetAgentStage: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentStage204Response); !ok {
		t.Fatalf("response = %T", resp)
	}
	if stages.set != domain.StageDraft {
		t.Errorf("stage = %q", stages.set)
	}
}

func TestSetAgentStage_withoutTheAuthorityToCauseRuns_isForbidden(t *testing.T) {
	t.Parallel()

	// Promoting is causing every run this agent makes from now on that nobody
	// will be asked about. Reading definitions is not that.
	stages := &fakeStages{stage: domain.StageCopilot}
	resp, err := staging(t, stages, ledger.NewMemory()).
		SetAgentStage(inArea("cx", domain.RoleAuditor), toStage("autonomous"))
	if err != nil {
		t.Fatalf("SetAgentStage: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentStage403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want it refused", resp)
	}
	if stages.set != "" {
		t.Error("it was promoted anyway")
	}
}

func TestSetAgentStage_promotingACopilot_needsNoFreshSimulation(t *testing.T) {
	t.Parallel()

	// It left Draft once, and has been working under supervision since. The
	// gate is on leaving Draft, not on every promotion after it.
	stages := &fakeStages{stage: domain.StageCopilot}
	resp, err := staging(t, stages, ledger.NewMemory()).
		SetAgentStage(inArea("cx", domain.RoleAuthor), toStage("autonomous"))
	if err != nil {
		t.Fatalf("SetAgentStage: %v", err)
	}
	if _, ok := resp.(openapi.SetAgentStage204Response); !ok {
		t.Fatalf("response = %T", resp)
	}
	if stages.set != domain.StageAutonomous {
		t.Errorf("stage = %q", stages.set)
	}
}
