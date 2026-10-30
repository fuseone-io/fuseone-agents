package autonomy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/autonomy"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

type stub struct {
	agreements []domain.Agreement
	stages     map[domain.AgentID]domain.Stage
	set        map[domain.AgentID]domain.Stage
	recorded   []string
}

func (s *stub) Agreement(context.Context, time.Time) ([]domain.Agreement, error) {
	return s.agreements, nil
}

func (s *stub) Stages(context.Context) (map[domain.AgentID]domain.Stage, error) {
	return s.stages, nil
}

func (s *stub) SetStage(_ context.Context, a domain.AgentID, stage domain.Stage, _ domain.UserID) error {
	if s.set == nil {
		s.set = map[domain.AgentID]domain.Stage{}
	}
	s.set[a] = stage
	return nil
}

func (s *stub) Record(_ context.Context, a domain.AgentID, action string, _ any) error {
	s.recorded = append(s.recorded, string(a)+":"+action)
	return nil
}

func watching(s *stub) *autonomy.Watch { return autonomy.New(s, s, s, nil) }

// A copilot whose approvals all came from runs that fell back has shown
// nothing about its declared pair, which is what promotion would let it act
// alone on (#27). Read through the real store, so the exclusion is the one
// production has.
func TestSuggestions_approvalsFromRunsThatFellBack_promoteNothing(t *testing.T) {
	t.Parallel()

	// The control (false) proves the fixture can earn a suggestion at all.
	for fell, want := range map[bool]int{false: 1, true: 0} {
		store := ledger.NewMemory()
		now := time.Now()
		for i := range domain.PromoteAfter + 5 {
			approvedRun(t, store, domain.RunID(fmt.Sprintf("run-%d", i)), now, fell)
		}
		s := &stub{stages: map[domain.AgentID]domain.Stage{"suporte": domain.StageCopilot}}
		got, err := autonomy.New(store, s, s, nil).Suggestions(context.Background(), now)
		if err != nil {
			t.Fatalf("Suggestions: %v", err)
		}
		if len(got) != want {
			t.Errorf("fell back = %v: suggestions = %+v, want %d", fell, got, want)
		}
	}
}

func approvedRun(t *testing.T, store *ledger.Memory, id domain.RunID, at time.Time, fell bool) {
	t.Helper()
	steps := []struct {
		kind    domain.StepKind
		payload any
	}{{domain.StepRunStarted, domain.RunStartedPayload{Trigger: "manual"}}}
	if fell {
		steps = append(steps, struct {
			kind    domain.StepKind
			payload any
		}{domain.StepModelFallback, domain.ModelFallbackPayload{
			To: domain.ModelPair{Provider: "openai", Model: "gpt-test"},
		}})
	}
	steps = append(steps, struct {
		kind    domain.StepKind
		payload any
	}{domain.StepApprovalDecided, domain.ApprovalDecidedPayload{Approved: true, By: "ana"}})
	for _, s := range steps {
		raw, err := json.Marshal(s.payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := store.Append(context.Background(), domain.Step{
			RunID: id, Kind: s.kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "suporte", VersionID: "v1", At: at.Add(-time.Minute), Payload: raw,
		}); err != nil {
			t.Fatalf("append %s: %v", s.kind, err)
		}
	}
}

func TestSweep_demotesAnAutonomousAgentPeopleKeepOverruling(t *testing.T) {
	t.Parallel()

	s := &stub{
		agreements: []domain.Agreement{{Agent: "suporte", Approved: 2, Refused: 4}},
		stages:     map[domain.AgentID]domain.Stage{"suporte": domain.StageAutonomous},
	}
	demoted, err := watching(s).Sweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	// One step, to copilot: the agent keeps working and a person sees each
	// action. Switching it off over a statistic is a platform nobody leaves
	// switched on.
	if demoted != 1 || s.set["suporte"] != domain.StageCopilot {
		t.Fatalf("demoted %d to %q", demoted, s.set["suporte"])
	}
	// The platform acting on its own is the change somebody comes looking for
	// an explanation of.
	if len(s.recorded) != 1 {
		t.Errorf("trail = %v", s.recorded)
	}
}

func TestSweep_leavesACopilotAlone(t *testing.T) {
	t.Parallel()

	// It is already asking about everything. Demoting it to draft would stop
	// the work over a rate somebody may be about to explain.
	s := &stub{
		agreements: []domain.Agreement{{Agent: "suporte", Approved: 1, Refused: 9}},
		stages:     map[domain.AgentID]domain.Stage{"suporte": domain.StageCopilot},
	}
	demoted, _ := watching(s).Sweep(context.Background(), time.Now())

	if demoted != 0 || len(s.set) != 0 {
		t.Errorf("demoted %d, set %v", demoted, s.set)
	}
}

func TestSuggestions_neverPromotesAnything(t *testing.T) {
	t.Parallel()

	s := &stub{
		agreements: []domain.Agreement{{Agent: "suporte", Approved: 40}},
		stages:     map[domain.AgentID]domain.Stage{"suporte": domain.StageCopilot},
	}
	got, err := watching(s).Suggestions(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("Suggestions: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("suggestions = %+v", got)
	}
	// An agent that promoted itself for agreeing with people is an agent that
	// stops being asked about.
	if len(s.set) != 0 {
		t.Errorf("it promoted something: %v", s.set)
	}
}

func TestSuggestions_neverSuggestsPromotingADraft(t *testing.T) {
	t.Parallel()

	// A draft has not been simulated and reviewed, and that gate is a
	// person's too (FU-10). Agreement cannot substitute for it.
	s := &stub{
		agreements: []domain.Agreement{{Agent: "novo", Approved: 40}},
		stages:     map[domain.AgentID]domain.Stage{"novo": domain.StageDraft},
	}
	got, _ := watching(s).Suggestions(context.Background(), time.Now())

	if len(got) != 0 {
		t.Errorf("suggestions = %+v, want none", got)
	}
}
