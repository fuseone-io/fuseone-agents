package guard

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// session connects a real MCP client to the guard's server for role, over the
// SDK's in-memory transport, with token as the GitHub credential.
func (h *harness) session(role Role, token string) *mcp.ClientSession {
	h.t.Helper()
	cfg := h.cfg
	cfg.Role = role
	server, err := NewServer(cfg, h.client(token))
	if err != nil {
		h.t.Fatalf("new server: %v", err)
	}
	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := server.Connect(h.t.Context(), serverT, nil)
	if err != nil {
		h.t.Fatalf("connect server: %v", err)
	}
	h.t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "fuseone", Version: "test"}, nil)
	cs, err := client.Connect(h.t.Context(), clientT, nil)
	if err != nil {
		h.t.Fatalf("connect client: %v", err)
	}
	h.t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func (h *harness) call(cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	h.t.Helper()
	res, err := cs.CallTool(h.t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		h.t.Fatalf("call %s: %v", tool, err)
	}
	return res
}

func structured[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("structured content %s: %v", raw, err)
	}
	return out
}

func TestRecordQAVerdictTool_recorded_returnsStructuredSuccess(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	cs := h.session(RoleQA, "qa-token")
	res := h.call(cs, "record_qa_verdict", map[string]any{
		"pr": testPR, "head_sha": headA, "verdict": "success", "summary": "criteria met",
	})
	got := structured[VerdictResult](t, res)
	if res.IsError || !got.Recorded || got.Reason != ReasonRecorded {
		t.Fatalf("isError %v, result %+v", res.IsError, got)
	}
}

// A refusal is a failed tool call, so the run sees it as one, and still
// carries the structured reason rather than prose.
func TestRecordQAVerdictTool_refused_isAnErrorWithStructuredReason(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.edit(func(f *fakeGitHub) { f.prs[testPR].Head = headB })
	cs := h.session(RoleQA, "qa-token")
	res := h.call(cs, "record_qa_verdict", map[string]any{
		"pr": testPR, "head_sha": headA, "verdict": "success", "summary": "criteria met",
	})
	got := structured[VerdictResult](t, res)
	if !res.IsError || got.Reason != ReasonHeadMoved {
		t.Fatalf("isError %v, result %+v, want an error carrying head_moved", res.IsError, got)
	}
}
