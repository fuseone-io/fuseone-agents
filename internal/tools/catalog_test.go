package tools_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/known"
	"github.com/fuseone/agents/internal/tools"
)

// fakeServer stands in for a connected MCP server.
type fakeServer struct {
	list    []*mcp.Tool
	listErr error
	result  *mcp.CallToolResult
	err     error
	calls   []string
	closed  bool
}

func (f *fakeServer) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &mcp.ListToolsResult{Tools: f.list}, nil
}

func (f *fakeServer) CallTool(_ context.Context, p *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	f.calls = append(f.calls, p.Name)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeServer) Close() error { f.closed = true; return nil }

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func catalogWith(t *testing.T, srv *fakeServer) (*tools.Catalog, *engine.MemoryContent) {
	t.Helper()

	content := engine.NewMemoryContent()
	c := tools.NewCatalog(content)
	if err := c.AddServer(context.Background(), "crm", srv, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	return c, content
}

func mergeServer() *fakeServer {
	return &fakeServer{
		list: []*mcp.Tool{{
			Name:        "merge_pull_request",
			Description: "Merge a pull request.",
			InputSchema: map[string]any{"type": "object"},
		}},
		result: text("merged"),
	}
}

func lookupServer() *fakeServer {
	return &fakeServer{
		list: []*mcp.Tool{{
			Name:        "lookup",
			Description: "Look a customer up by email.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"email": map[string]any{"type": "string"}},
			},
		}},
		result: text("Cliente encontrado: ACME Ltda"),
	}
}

/*
An imported tool arrives unclassified, and unclassified does not execute.

It used to arrive as READ, which reads like a restriction and is a permission:
READ is allowed outright, so a server offering forty tools created forty the
Gate would let through — `delete_repository` among them — until somebody ruled
on each by name. The label was a claim about the tool and nothing verified it.

The server still must not be able to grant itself anything by describing a tool
as one; that argument was always right and it argues for unclassified, which
refuses the server's claim without acting on it either way.
*/
func TestAddServer_importedTool_arrivesUnclassifiedAndUntrusted(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())

	effect, ok := c.Effect("crm.lookup")
	if !ok {
		t.Fatal("the imported tool is not in the catalogue")
	}
	if effect != domain.EffectUnknown {
		t.Errorf("Effect = %v, want unknown until the Curator rules", effect)
	}
}

func TestEffect_unknownTool_reportsUnknownSoTheGateBlocks(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())

	// The Gate's first check is capability, and its second is classification.
	// A tool nobody classified must never resolve to a usable effect.
	if _, ok := c.Effect("crm.refund"); ok {
		t.Error("an unregistered tool resolved to an effect")
	}
}

func TestAddServer_toolsAreNamespacedByServer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	content := engine.NewMemoryContent()
	c := tools.NewCatalog(content)

	// Two servers both offering "lookup" must not collapse into one
	// capability — a pack granting one would silently grant the other.
	if err := c.AddServer(ctx, "crm", lookupServer(), nil); err != nil {
		t.Fatalf("AddServer(crm): %v", err)
	}
	if err := c.AddServer(ctx, "erp", lookupServer(), nil); err != nil {
		t.Fatalf("AddServer(erp): %v", err)
	}

	if _, ok := c.Effect("crm.lookup"); !ok {
		t.Error("crm.lookup missing")
	}
	if _, ok := c.Effect("erp.lookup"); !ok {
		t.Error("erp.lookup missing")
	}
}

func TestClassify_curatorWidensEffect(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())

	if err := c.Classify(domain.ToolClassification{Tool: "crm.lookup", Effect: domain.EffectWrite, Untrusted: true}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if effect, _ := c.Effect("crm.lookup"); effect != domain.EffectWrite {
		t.Errorf("Effect = %v, want write after the Curator classified it", effect)
	}
}

func TestClassify_unknownTool_isRejected(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())

	if err := c.Classify(domain.ToolClassification{Tool: "crm.nope", Effect: domain.EffectWrite, Untrusted: false}); !errors.Is(err, tools.ErrUnknownTool) {
		t.Errorf("Classify = %v, want %v", err, tools.ErrUnknownTool)
	}
}

func TestInvoke_untrustedServer_taintsTheResult(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())

	res, err := c.Invoke(context.Background(), engine.Call{
		RunID: "run-1", Seq: 5, Tool: "crm.lookup", Args: []byte(`{"email":"a@b.com"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// This label is what makes taint propagate through the run. Without it,
	// content the platform never vouched for reads as trusted downstream.
	if !res.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("Labels = %v, want the untrusted label", res.Labels)
	}
}

func TestInvoke_vouchedServer_returnsUntaintedResults(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())
	if err := c.Classify(domain.ToolClassification{Tool: "crm.lookup", Effect: domain.EffectRead, Untrusted: false}); err != nil {
		t.Fatalf("Classify: %v", err)
	}

	res, err := c.Invoke(context.Background(), engine.Call{
		RunID: "run-1", Seq: 5, Tool: "crm.lookup",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Labels.Has(domain.LabelUntrusted) {
		t.Error("a server the Curator vouched for still returns tainted output")
	}
}

func TestInvoke_result_goesToContentStoreNotTheLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	c, content := catalogWith(t, lookupServer())

	res, err := c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 5, Tool: "crm.lookup"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Tool output routinely carries personal data. The ledger records a
	// reference so retention stays honourable (PRD AU-04).
	if res.ResultRef == "" {
		t.Fatal("no content reference returned")
	}
	stored, err := content.Get(ctx, res.ResultRef)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(stored) != "Cliente encontrado: ACME Ltda" {
		t.Errorf("stored = %q, want the tool's output", stored)
	}
}

func TestInvoke_toolReportsFailure_isSurfacedNotSwallowed(t *testing.T) {
	t.Parallel()

	srv := lookupServer()
	srv.result = &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "customer not found"}},
	}
	c, _ := catalogWith(t, srv)

	res, err := c.Invoke(context.Background(), engine.Call{RunID: "run-1", Seq: 5, Tool: "crm.lookup"})
	if err != nil {
		t.Fatalf("Invoke returned a transport error for a tool-level failure: %v", err)
	}
	// A tool that reports failure is a result, not a transport error — the
	// model needs to see it to choose a different approach.
	if !res.Failed {
		t.Error("Failed = false for a tool that reported an error")
	}
}

func TestInvoke_transportFailureIsStoredForTheModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srv := lookupServer()
	srv.err = errors.New("Bad Request: Unsupported protocol version: 2026-07-28")
	c, content := catalogWith(t, srv)

	res, err := c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 5, Tool: "crm.lookup"})
	if err == nil {
		t.Fatal("Invoke succeeded despite a transport failure")
	}
	if !res.Failed || res.ErrorCode != "invoke_error" {
		t.Fatalf("failure = (%v, %q), want invoke_error", res.Failed, res.ErrorCode)
	}
	if res.ResultRef == "" {
		t.Fatal("transport failure did not produce a content reference")
	}
	if !res.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("Labels = %v, want the untrusted label on a failed untrusted tool", res.Labels)
	}

	stored, err := content.Get(ctx, res.ResultRef)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got := string(stored)
	for _, want := range []string{
		"the tool failed: invoke_error",
		"crm.lookup",
		"Unsupported protocol version: 2026-07-28",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stored diagnostic = %q, want it to contain %q", got, want)
		}
	}
}

func TestInvoke_unknownTool_neverReachesAServer(t *testing.T) {
	t.Parallel()

	srv := lookupServer()
	c, _ := catalogWith(t, srv)

	_, err := c.Invoke(context.Background(), engine.Call{RunID: "run-1", Tool: "crm.refund"})
	if !errors.Is(err, tools.ErrUnknownTool) {
		t.Errorf("Invoke = %v, want %v", err, tools.ErrUnknownTool)
	}
	if len(srv.calls) != 0 {
		t.Errorf("an unregistered tool reached the server: %v", srv.calls)
	}
}

func TestInvoke_malformedArguments_areRejectedBeforeTheCall(t *testing.T) {
	t.Parallel()

	srv := lookupServer()
	c, _ := catalogWith(t, srv)

	_, err := c.Invoke(context.Background(), engine.Call{
		RunID: "run-1", Tool: "crm.lookup", Args: []byte(`{not json`),
	})
	if err == nil {
		t.Fatal("malformed arguments were accepted")
	}
	if len(srv.calls) != 0 {
		t.Error("a malformed call still reached the server")
	}
}

func TestSchema_describesTheToolToTheModel(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, lookupServer())

	name, desc, schema, ok := c.Schema("crm.lookup")
	if !ok {
		t.Fatal("no schema for a registered tool")
	}
	// The namespaced id is what the model calls, so a proposal maps back to
	// exactly one catalogue entry.
	if name != "crm.lookup" {
		t.Errorf("name = %q, want the namespaced id", name)
	}
	if desc == "" {
		t.Error("no description; the model has nothing to decide from")
	}
	if _, has := schema["email"]; !has {
		t.Errorf("schema = %v, want the tool's own parameters", schema)
	}
}

func TestClose_shutsEverySessionDown(t *testing.T) {
	t.Parallel()

	srv := lookupServer()
	c, _ := catalogWith(t, srv)

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !srv.closed {
		t.Error("the session was left open")
	}
}

// staticRulings stands in for the administration's record of what tools do.
type staticRulings []domain.ToolClassification

func (s staticRulings) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	return s, nil
}

func TestSync_appliesARecordedRuling_soAPromotionOutlivesTheProcess(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, noteServer())

	// Imported unclassified, as every tool is, whatever its server claims.
	if effect, _ := c.Effect("crm.note"); effect != domain.EffectUnknown {
		t.Fatalf("imported effect = %v, want unknown", effect)
	}

	applied, err := c.Sync(t.Context(), staticRulings{
		{Tool: "crm.note", Effect: domain.EffectWrite, Untrusted: false},
	}, domain.Scope{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if effect, _ := c.Effect("crm.note"); effect != domain.EffectWrite {
		t.Errorf("effect after sync = %v, want write", effect)
	}
}

func TestSync_rulingForAToolThisCatalogueLacks_isIgnoredNotFatal(t *testing.T) {
	t.Parallel()

	c, _ := catalogWith(t, noteServer())

	// Servers come and go. A stale ruling for an absent tool must not stop
	// every current ruling from being applied.
	applied, err := c.Sync(t.Context(), staticRulings{
		{Tool: "gone.tool", Effect: domain.EffectWrite},
		{Tool: "crm.note", Effect: domain.EffectWrite},
	}, domain.Scope{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want only the tool this catalogue carries", applied)
	}
}

func noteServer() *fakeServer {
	return &fakeServer{
		list: []*mcp.Tool{{
			Name:        "note",
			Description: "Record an internal note.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
			},
		}},
		result: text("ok"),
	}
}

func TestRemoveServer_takesItsToolsWithIt(t *testing.T) {
	t.Parallel()

	catalog := tools.NewCatalog(engine.NewMemoryContent())
	crm := &fakeServer{list: []*mcp.Tool{{Name: "lookup"}, {Name: "note"}}}
	if err := catalog.AddServer(t.Context(), "crm", crm, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if err := catalog.AddServer(t.Context(), "kb", &fakeServer{list: []*mcp.Tool{{Name: "search"}}}, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	if err := catalog.RemoveServer("crm"); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}

	// A tool from a server nobody is connected to any more would be proposed,
	// gated, and then fail at the call — a refusal the trail explains badly.
	if _, ok := catalog.Effect("crm.lookup"); ok {
		t.Error("a removed server's tool is still in the catalogue")
	}
	// And only that server's: the others are still connected.
	if _, ok := catalog.Effect("kb.search"); !ok {
		t.Error("removing one server took another's tools with it")
	}
	// The session is closed rather than left open: a process this
	// installation started keeps running until somebody stops it.
	if !crm.closed {
		t.Error("the removed server's session was left open")
	}
}

func TestRemoveServer_thatWasNeverConnected_isNotAnError(t *testing.T) {
	t.Parallel()

	// The reconciler asks for removal from a desired state, not from
	// knowledge of what is connected. Refusing here would make an ordinary
	// pass log an error nobody can act on.
	if err := tools.NewCatalog(engine.NewMemoryContent()).RemoveServer("nunca"); err != nil {
		t.Errorf("RemoveServer: %v", err)
	}
}

func TestAddServer_replacingAServerRemovesToolsItNoLongerOffers(t *testing.T) {
	t.Parallel()

	catalog := tools.NewCatalog(engine.NewMemoryContent())
	first := &fakeServer{list: []*mcp.Tool{{
		Name:        "lookup",
		Description: "Look a customer up.",
		InputSchema: map[string]any{"type": "object"},
	}}}
	if err := catalog.AddServer(t.Context(), "crm", first, nil); err != nil {
		t.Fatalf("AddServer(first): %v", err)
	}

	second := &fakeServer{list: []*mcp.Tool{{
		Name:        "search",
		Description: "Search customers.",
		InputSchema: map[string]any{"type": "object"},
	}}}
	if err := catalog.AddServer(t.Context(), "crm", second, nil); err != nil {
		t.Fatalf("AddServer(second): %v", err)
	}

	if !first.closed {
		t.Error("the replaced session was not closed")
	}
	if _, known := catalog.Effect("crm.lookup"); known {
		t.Error("the old tool survived the replacement")
	}
	if _, known := catalog.Effect("crm.search"); !known {
		t.Error("the new tool was not imported")
	}
}

func TestAddServer_aFailedReplacementKeepsTheCurrentServer(t *testing.T) {
	t.Parallel()

	catalog := tools.NewCatalog(engine.NewMemoryContent())
	first := &fakeServer{list: []*mcp.Tool{{
		Name:        "lookup",
		Description: "Look a customer up.",
		InputSchema: map[string]any{"type": "object"},
	}}}
	if err := catalog.AddServer(t.Context(), "crm", first, nil); err != nil {
		t.Fatalf("AddServer(first): %v", err)
	}

	second := &fakeServer{listErr: errors.New("server is waking up")}
	if err := catalog.AddServer(t.Context(), "crm", second, nil); err == nil {
		t.Fatal("AddServer(second) succeeded, want the discovery error")
	}

	if first.closed {
		t.Error("the live session was closed by a failed replacement")
	}
	if _, known := catalog.Effect("crm.lookup"); !known {
		t.Error("the live tool disappeared after a failed replacement")
	}
}

/*
A tool the platform already knows something about.

Unclassified is the right default and it is also forty rulings to write by hand
before a well-known server does anything, which is how a safe default becomes
one somebody works around. So a shipped suggestion travels with the tool.

It is a suggestion and stays one. Applied on import it would put the decision
back in a table shipped in a binary — the same mistake as trusting the server,
one step further away and harder to see.
*/
func TestAddServer_aKnownServer_carriesASuggestionAndStaysUnclassified(t *testing.T) {
	t.Parallel()

	known, err := known.Load()
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	c, _ := catalogWith(t, mergeServer())
	c.Knowing(known)
	if err := c.AddServer(t.Context(), "github", mergeServer(), nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	entry, ok := c.Lookup("github.merge_pull_request")
	if !ok {
		t.Fatal("the imported tool is not in the catalogue")
	}
	if entry.Effect != domain.EffectUnknown {
		t.Errorf("Effect = %v, want it still unclassified", entry.Effect)
	}
	if entry.Suggested == nil || entry.Suggested.Effect != domain.EffectDestructive {
		t.Errorf("Suggested = %+v, want destructive suggested", entry.Suggested)
	}
}

// A server nobody catalogued imports exactly as before. The suggestion is an
// addition and never a precondition.
func TestAddServer_anUnknownServer_importsWithNoSuggestion(t *testing.T) {
	t.Parallel()

	known, err := known.Load()
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	c, _ := catalogWith(t, lookupServer())
	c.Knowing(known)
	if err := c.AddServer(t.Context(), "acme", lookupServer(), nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	entry, _ := c.Lookup("acme.lookup")
	if entry.Suggested != nil {
		t.Errorf("Suggested = %+v, want nothing for a server nobody catalogued", entry.Suggested)
	}
}
