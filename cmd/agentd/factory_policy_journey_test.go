package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The Merger's merge reaches a person only through the policy the setup texts
author (#199). merge_if_ready is destructive: with no policy, the Gate blocks
it, and an approval never releases a block.

This stores the policy exactly as factory/agents/README.md gives it, through
the API the console writes with, refreshes the worker's enforcer from the
store, and asks the Gate. internal/spec holds every setup text to the same
payload.
*/

// mergePolicyRequest opens the policy's block in factory/agents/README.md.
const mergePolicyRequest = "PUT /api/v1/policies/FACTORY-MERGE"

// mergePolicyBody is the README's payload: from the first line that is
// exactly "{" after the request line, to the fence that closes the block.
func mergePolicyBody(t *testing.T) openapi.PolicyInput {
	t.Helper()
	raw, err := os.ReadFile("../../factory/agents/README.md")
	if err != nil {
		t.Fatalf("read the README: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	start := slices.Index(lines, mergePolicyRequest)
	if start < 0 {
		t.Fatalf("the README has no line %q", mergePolicyRequest)
	}
	body := lines[start:]
	open, end := slices.Index(body, "{"), slices.Index(body, "```")
	if open < 0 || end < open {
		t.Fatal("the README's merge policy has no body")
	}
	var in openapi.PolicyInput
	if err := json.Unmarshal([]byte(strings.Join(body[open:end], "\n")), &in); err != nil {
		t.Fatalf("the README's merge policy does not decode: %v", err)
	}
	return in
}

// mergerEnforcer is the worker's Gate after the policy was stored.
func mergerEnforcer(t *testing.T) *policy.Enforcer {
	t.Helper()
	store := policy.NewStore(testdb.Pool(t))
	if _, err := store.Delete(t.Context(), "FACTORY-MERGE", domain.IfMatch{}); err != nil {
		t.Fatalf("reset the policy: %v", err)
	}
	api := httpapi.NewServer(ledger.NewMemory(), "test").WithPolicies(store)
	body := mergePolicyBody(t)
	resp, err := api.PutPolicy(installer(t), openapi.PutPolicyRequestObject{Code: "FACTORY-MERGE", Body: &body})
	if _, ok := resp.(openapi.PutPolicy200JSONResponse); err != nil || !ok {
		t.Fatalf("store the merge policy = %#v, %v; want 200", resp, err)
	}
	enforcer := policy.NewEnforcer(store, slog.New(slog.DiscardHandler))
	if err := enforcer.Refresh(t.Context()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	return enforcer
}

// mergerCall is a copilot Merger run proposing a destructive call.
func mergerCall(agent domain.AgentID, tool domain.ToolID, granted bool) gate.Request {
	return gate.Request{
		Scope: domain.Scope{Company: "acme", Area: "factory"}, RunID: "run-merge", AgentID: agent, Seq: 3,
		Tool: tool, Effect: domain.EffectDestructive, Args: []byte(`{}`),
		Pack:   gate.NewPack("factory-merger.merge_if_ready", "factory-merger.delete_branch", "factory-developer.merge_if_ready"),
		Budget: domain.Budget{Micros: 200_000, ToolCalls: 12, Steps: 24}, Estimate: domain.Consumption{ToolCalls: 1},
		IdemKey: "run-merge:3:" + string(tool), Stage: domain.StageCopilot, ApprovalGranted: granted,
	}
}

func TestFactoryPolicy_mergeIfReady_reachesAPersonAndTheirApprovalReleasesIt(t *testing.T) {
	enforcer := mergerEnforcer(t)

	proposed, err := enforcer.Evaluate(t.Context(), mergerCall("factory-merger", "factory-merger.merge_if_ready", false))
	if err != nil || proposed.Verdict != domain.VerdictRequireApproval || proposed.PolicyCode != "FACTORY-MERGE" {
		t.Fatalf("the Merger's merge = %+v, %v; want require_approval by FACTORY-MERGE", proposed, err)
	}
	approved, err := enforcer.Evaluate(t.Context(), mergerCall("factory-merger", "factory-merger.merge_if_ready", true))
	if err != nil || approved.Verdict != domain.VerdictAllow {
		t.Fatalf("the approved merge = %+v, %v; want allow", approved, err)
	}
}

// The policy loosens nothing else: another destructive tool, or the same
// tool from another agent, stays blocked, approval or not.
func TestFactoryPolicy_anyOtherDestructiveCall_staysBlocked(t *testing.T) {
	enforcer := mergerEnforcer(t)

	for name, call := range map[string]gate.Request{
		"another destructive tool of the Merger": mergerCall("factory-merger", "factory-merger.delete_branch", true),
		"the same tool from another agent":       mergerCall("factory-developer", "factory-merger.merge_if_ready", true),
		"a same-named tool on another server":    mergerCall("factory-merger", "factory-developer.merge_if_ready", true),
	} {
		got, err := enforcer.Evaluate(t.Context(), call)
		if err != nil || got.Verdict != domain.VerdictBlock {
			t.Errorf("%s = %+v, %v; want block", name, got, err)
		}
	}
}
