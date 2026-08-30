package connectortools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/contextshare"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/memory"
)

func TestSQLActivation_eachExecutionCrossesApprovalAndReceivesFreshAuthority(t *testing.T) {
	t.Parallel()

	issuer := &rotatingSQLIssuer{}
	executor := &recordingSQLExecutor{}
	base := &cachingBase{}
	layer := newSQLLayer(t, issuer, executor, base)
	store := ledger.NewMemory()
	content := layer.content
	contextTools := contextshare.New(layer, layer, content)
	tools := memory.NewLayer(contextTools, contextTools, content, memory.NewMemory())
	tool := domain.ToolID("sql.app-x.run_query_template")
	planner := &repeatingSQLPlanner{tool: tool, args: sqlLayerCall(1).Args}
	runner := engine.NewRunner(engine.Deps{
		Ledger: store, Content: content, Planner: planner,
		Tools: tools, Catalog: tools, Clock: engine.SystemClock{},
		Gate: gate.New().WithPolicies(gate.Policies{
			Hash: "pol_sql_approval",
			Set: []domain.Policy{{
				Code: "POL-SQL", Name: "Approve production SQL", Enabled: true,
				Mode: domain.PolicyEnforce, Effect: domain.PolicyEscalate,
				Resource: string(tool), Reach: domain.ReachInstallation,
			}},
		}),
	})
	start := engine.Start{
		RunID: "run-sql-approval", Scope: runScope(), AgentID: "operator",
		VersionID: "v1", OnBehalfOf: "ana", Pack: gate.NewPack(tool),
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 10, Steps: 30},
		Stage:  domain.StageAutonomous, Trigger: "test",
	}

	first, err := runner.Advance(context.Background(), start)
	if err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	if first.Phase != engine.PhaseAwaitingApproval {
		t.Fatalf("first phase = %v, want approval", first.Phase)
	}
	wantNoSQLExecution(t, issuer, executor)

	approveSQL(t, store, start)
	if _, err := runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("approved Advance: %v", err)
	}
	wantSQLExecutions(t, issuer, executor, 1)

	second, err := runner.Advance(context.Background(), start)
	if err != nil {
		t.Fatalf("repeat Advance: %v", err)
	}
	if second.Phase != engine.PhaseAwaitingApproval {
		t.Fatalf("repeat phase = %v, want a fresh approval", second.Phase)
	}
	wantSQLExecutions(t, issuer, executor, 1)

	approveSQL(t, store, start)
	if _, err := runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("second approved Advance: %v", err)
	}
	wantSQLExecutions(t, issuer, executor, 2)
	if base.invoked != 0 {
		t.Fatalf("SQL reached the MCP cache %d times", base.invoked)
	}

	steps, err := store.Read(context.Background(), start.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	leaks(t, steps)
	for _, step := range steps {
		for _, ref := range contentRefs(t, step) {
			body, err := content.Get(context.Background(), ref)
			if err != nil {
				t.Fatalf("read %s content: %v", step.Kind, err)
			}
			leaks(t, body)
		}
	}
}

func TestSQLActivation_changingTheRegisteredQueryRequiresAnotherApproval(t *testing.T) {
	t.Parallel()

	issuer := &rotatingSQLIssuer{}
	executor := &recordingSQLExecutor{}
	layer := newSQLLayer(t, issuer, executor, &cachingBase{})
	store := ledger.NewMemory()
	contextTools := contextshare.New(layer, layer, layer.content)
	tools := memory.NewLayer(contextTools, contextTools, layer.content, memory.NewMemory())
	tool := domain.ToolID("sql.app-x.run_query_template")
	runner := engine.NewRunner(engine.Deps{
		Ledger: store, Content: layer.content,
		Planner: &repeatingSQLPlanner{tool: tool, args: sqlLayerCall(1).Args},
		Tools:   tools, Catalog: tools, Clock: engine.SystemClock{},
		Gate: gate.New().WithPolicies(gate.Policies{Hash: "pol_sql_approval", Set: []domain.Policy{{
			Code: "POL-SQL", Name: "Approve production SQL", Enabled: true,
			Mode: domain.PolicyEnforce, Effect: domain.PolicyEscalate,
			Resource: string(tool), Reach: domain.ReachInstallation,
		}}}),
	})
	start := engine.Start{
		RunID: "run-sql-contract-change", Scope: runScope(), AgentID: "operator",
		VersionID: "v1", OnBehalfOf: "ana", Pack: gate.NewPack(tool),
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 10, Steps: 30},
		Stage:  domain.StageAutonomous, Trigger: "test",
	}

	if _, err := runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	before := approvalContracts(t, store, start.RunID)
	if len(before) != 1 || before[0] == "" {
		t.Fatalf("approval contracts = %#v, want the registered query pinned", before)
	}

	changed := ready()
	changed[1].SQL.Templates[0].SQL += " and 1 = 1"
	if err := layer.SetInstances(changed); err != nil {
		t.Fatalf("change registered template: %v", err)
	}
	approveSQL(t, store, start)
	status, err := runner.Advance(context.Background(), start)
	if err != nil {
		t.Fatalf("Advance after stale approval: %v", err)
	}
	if status.Phase != engine.PhaseAwaitingApproval {
		t.Fatalf("phase = %v, want another approval", status.Phase)
	}
	wantNoSQLExecution(t, issuer, executor)

	after := approvalContracts(t, store, start.RunID)
	if len(after) != 2 || after[1] == "" || after[0] == after[1] {
		t.Fatalf("approval contracts = %#v, want old and new query identities", after)
	}
}

type repeatingSQLPlanner struct {
	tool  domain.ToolID
	args  []byte
	calls int
}

func (p *repeatingSQLPlanner) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	p.calls++
	if p.calls <= 2 {
		return engine.Proposal{Tool: p.tool, Args: p.args}, nil
	}
	return engine.Proposal{Done: true, Outcome: "done"}, nil
}

func approveSQL(t *testing.T, store engine.Ledger, start engine.Start) {
	t.Helper()
	payload, err := json.Marshal(domain.ApprovalDecidedPayload{Approved: true, By: "ana"})
	if err != nil {
		t.Fatalf("marshal approval: %v", err)
	}
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: start.RunID, Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Kind: domain.StepApprovalDecided, At: time.Now(), Payload: payload,
	}); err != nil {
		t.Fatalf("append approval: %v", err)
	}
}

func wantNoSQLExecution(t *testing.T, issuer *rotatingSQLIssuer, executor *recordingSQLExecutor) {
	t.Helper()
	wantSQLExecutions(t, issuer, executor, 0)
}

func wantSQLExecutions(
	t *testing.T, issuer *rotatingSQLIssuer, executor *recordingSQLExecutor, want int,
) {
	t.Helper()
	issued, revoked := issuer.snapshot()
	credentials := executor.snapshot()
	if len(issued) != want || len(revoked) != want || len(credentials) != want {
		t.Fatalf("executions = issued:%d revoked:%d opened:%d, want %d each",
			len(issued), len(revoked), len(credentials), want)
	}
	if want == 2 && (issued[0] == issued[1] || credentials[0] == credentials[1]) {
		t.Fatalf("authority was reused: leases=%#v credentials=%#v", issued, credentials)
	}
}

func contentRefs(t *testing.T, step domain.Step) []string {
	t.Helper()
	switch step.Kind {
	case domain.StepToolCalled:
		var payload domain.ToolCalledPayload
		if err := json.Unmarshal(step.Payload, &payload); err != nil {
			t.Fatalf("decode tool call: %v", err)
		}
		return []string{payload.ArgsRef}
	case domain.StepToolReturned:
		var payload domain.ToolReturnedPayload
		if err := json.Unmarshal(step.Payload, &payload); err != nil {
			t.Fatalf("decode tool result: %v", err)
		}
		return []string{payload.ResultRef}
	default:
		return nil
	}
}

func approvalContracts(t *testing.T, store engine.Ledger, runID domain.RunID) []string {
	t.Helper()
	steps, err := store.Read(context.Background(), runID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read approvals: %v", err)
	}
	var out []string
	for _, step := range steps {
		if step.Kind != domain.StepApprovalRequested {
			continue
		}
		var payload domain.ApprovalRequestedPayload
		if err := json.Unmarshal(step.Payload, &payload); err != nil {
			t.Fatalf("decode approval: %v", err)
		}
		out = append(out, payload.ContractDigest)
	}
	return out
}
