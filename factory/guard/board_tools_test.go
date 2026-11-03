package guard

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// session connects an MCP client to role's server over the board fake, with
// the role's own token.
func (h *boardHarness) session(role Role) *mcp.ClientSession {
	h.t.Helper()
	server, err := NewServer(Config{Board: h.cfg}, role, h.client(staticToken(roleToken(role))))
	if err != nil {
		h.t.Fatalf("new server: %v", err)
	}
	return connect(h.t, server)
}

func (h *boardHarness) requests() int {
	h.fake.mu.Lock()
	defer h.fake.mu.Unlock()
	return len(h.fake.tokens)
}

func callRaw(t *testing.T, cs *mcp.ClientSession, tool string, args any) (*mcp.CallToolResult, error) {
	t.Helper()
	return cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
}

// Any argument beyond the tool's own, a missing one, a non-string one or a
// key spelled in another case is invalid_input, and nothing is read or
// written: the tool takes no project, field or value from its caller.
func TestBoardTools_argumentsNotExactlyTheirOwn_invalidInput(t *testing.T) {
	t.Parallel()

	move := func(extra map[string]any) map[string]any {
		args := map[string]any{"item_id": testItem, "lane": "Preparado"}
		for k, v := range extra {
			args[k] = v
		}
		return args
	}
	cases := map[string]struct {
		tool string
		args any
	}{
		"move field_id":     {ToolMoveCard, move(map[string]any{"field_id": testWorkTypeField})},
		"move project_id":   {ToolMoveCard, move(map[string]any{"project_id": otherProjectID})},
		"move value":        {ToolMoveCard, move(map[string]any{"value": "opt-done"})},
		"move missing lane": {ToolMoveCard, map[string]any{"item_id": testItem}},
		"move lane number":  {ToolMoveCard, map[string]any{"item_id": testItem, "lane": 2}},
		"move other case":   {ToolMoveCard, map[string]any{"Item_ID": testItem, "lane": "Preparado"}},
		"move not object":   {ToolMoveCard, []string{testItem, "Preparado"}},
		"fields status":     {ToolSetCardFields, map[string]any{"item_id": testItem, "work_type": "bug", "size": "S", "status": "opt-done"}},
		"fields field_id":   {ToolSetCardFields, map[string]any{"item_id": testItem, "work_type": "bug", "size": "S", "field_id": testStatusField}},
		"fields missing":    {ToolSetCardFields, map[string]any{"item_id": testItem, "work_type": "bug"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, LaneTriagem)
			res, err := callRaw(t, h.session(RoleTriage), tc.tool, tc.args)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			got := structured[MoveResult](t, res)
			if n := h.requests(); !res.IsError || got.Reason != ReasonInvalidInput || n != 0 {
				t.Fatalf("isError %v, result %+v, %d requests: want invalid_input before any request", res.IsError, got, n)
			}
		})
	}
}

func TestMoveCardTool_isErrorOnlyWhenRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		from    Lane
		to      string
		isError bool
		reason  Reason
	}{
		"moved":           {LaneTriagem, "Preparado", false, ReasonMoved},
		"already in lane": {LaneTriagem, "Triagem", false, ReasonAlreadyInLane},
		"refused":         {LaneTravado, "Triagem", true, ReasonTravadoIsHuman},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newBoardHarness(t, tc.from)
			res := mustCall(t, h.session(RoleTriage), ToolMoveCard, map[string]any{"item_id": testItem, "lane": tc.to})
			if got := structured[MoveResult](t, res); res.IsError != tc.isError || got.Reason != tc.reason {
				t.Fatalf("isError %v, result %+v, want %v/%s", res.IsError, got, tc.isError, tc.reason)
			}
		})
	}
}

func mustCall(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := callRaw(t, cs, tool, args)
	if err != nil {
		t.Fatalf("call %s: %v", tool, err)
	}
	return res
}

func TestSetCardFieldsTool_updated_returnsStructuredSuccess(t *testing.T) {
	t.Parallel()

	h := newBoardHarness(t, LaneTriagem)
	res := mustCall(t, h.session(RoleArchitect), ToolSetCardFields,
		map[string]any{"item_id": testItem, "work_type": "chore", "size": "L"})
	if got := structured[FieldsResult](t, res); res.IsError || !got.Updated || len(got.Changed) != 2 {
		t.Fatalf("isError %v, result %+v", res.IsError, got)
	}
}

// refusedCall is a call the role's surface does not hold: the SDK answers
// it as an unknown tool, whichever way it reports that.
func refusedCall(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) bool {
	t.Helper()
	res, err := callRaw(t, cs, tool, args)
	return err != nil || res.IsError
}

// Every crossing #213 names, each with the credential and the state that
// would make the call succeed if the tool were exposed.
func TestNewServer_crossingIntoAnotherRolesTool_refusedWithoutEffect(t *testing.T) {
	t.Parallel()

	merge := map[string]any{"pr": testPR, "head_sha": headA}
	t.Run("qa calls merge_if_ready", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		if !refusedCall(t, h.session(RoleQA, "merger-token"), ToolMergeIfReady, merge) || len(h.fake.attempts) != 0 {
			t.Fatalf("QA reached the merge: %d attempts", len(h.fake.attempts))
		}
	})
	t.Run("reviewer calls merge_if_ready", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		if !refusedCall(t, h.session(RoleReviewer, "merger-token"), ToolMergeIfReady, merge) || len(h.fake.attempts) != 0 {
			t.Fatalf("the Reviewer reached the merge: %d attempts", len(h.fake.attempts))
		}
	})
	t.Run("triage calls record_qa_verdict", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		args := map[string]any{"pr": testPR, "head_sha": headA, "verdict": "success", "summary": "ok"}
		if !refusedCall(t, h.session(RoleTriage, "qa-token"), ToolRecordQAVerdict, args) || len(h.fake.posted) != 0 {
			t.Fatalf("Triage set a QA verdict: %d statuses", len(h.fake.posted))
		}
	})
	t.Run("developer calls set_card_fields", func(t *testing.T) {
		t.Parallel()
		h := newBoardHarness(t, LaneTriagem)
		args := map[string]any{"item_id": testItem, "work_type": "bug", "size": "S"}
		if !refusedCall(t, h.session(RoleDeveloper), ToolSetCardFields, args) || len(h.mutations()) != 0 {
			t.Fatalf("the Developer set a card's fields: %+v", h.mutations())
		}
	})
}
