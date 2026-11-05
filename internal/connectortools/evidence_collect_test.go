package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// fakeEvidenceSource answers the way a real source does: it pages what it
// holds, ignores nothing it was given, and records every request so a test can
// assert what reached it. A nil err and no hook is a healthy offset source.
type fakeEvidenceSource struct {
	events   int
	controls int
	requests []EvidenceRequest
	hook     func(EvidenceRequest) ([]byte, error, bool)
	row      func(i int) map[string]any
	seen     []string
}

func (f *fakeEvidenceSource) Fetch(
	_ context.Context, _ EvidenceConfig, credential SecretValue, request EvidenceRequest,
) ([]byte, error) {
	f.requests = append(f.requests, request)
	f.seen = append(f.seen, credential.value)
	if f.hook != nil {
		if body, err, handled := f.hook(request); handled {
			return body, err
		}
	}
	return f.page(request)
}

func (f *fakeEvidenceSource) page(request EvidenceRequest) ([]byte, error) {
	if request.Path == "/events/all" {
		return json.Marshal(map[string]any{"ok": 1, "result": make([]map[string]any, f.controls)})
	}
	offset, _ := strconv.Atoi(request.Query.Get("offset"))
	limit, _ := strconv.Atoi(request.Query.Get("limit"))
	rows := []map[string]any{}
	for i := offset; i < f.events && len(rows) < limit; i++ {
		rows = append(rows, f.rowAt(i))
	}
	return json.Marshal(map[string]any{"ok": 1, "result": rows})
}

func (f *fakeEvidenceSource) rowAt(i int) map[string]any {
	if f.row != nil {
		return f.row(i)
	}
	return map[string]any{
		"_id": fmt.Sprintf("e-%d", i), "timestamp": 1790000000 + i, "user": "a@example.com",
		"client_bytes": 10, "secret_field": "CANARY-undeclared",
	}
}

func evidenceLayer(t *testing.T, source *fakeEvidenceSource, edit ...func(*Instance)) *Layer {
	t.Helper()
	instance := validEvidenceInstance()
	for _, change := range edit {
		change(&instance)
	}
	layer := New(nil, nil, engine.NewMemoryContent(), nil).WithEvidence(source)
	if err := layer.SetInstances([]Instance{instance}); err != nil {
		t.Fatal(err)
	}
	return layer
}

func collectCall(args string) engine.Call {
	return engine.Call{
		Tool: "evidence.gateway.collect", RunID: "run-1", Seq: 4, Args: []byte(args),
		Scope: domain.Scope{Company: "acme", Area: "cyber"},
		At:    time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
	}
}

const byUserArgs = `{"query":"by_user","params":{"user":"a@example.com"},"window":{"periodSeconds":3600}}`

type collectAnswer struct {
	Operation  string `json:"operation"`
	Events     int64  `json:"events"`
	Unmeasured int64  `json:"unmeasured"`
	Coverage   string `json:"coverage"`
	Reason     string `json:"reason"`
	Evidence   string `json:"evidence"`
	Window     struct {
		Kind          string `json:"kind"`
		From          string `json:"from"`
		To            string `json:"to"`
		PeriodSeconds int    `json:"periodSeconds"`
	} `json:"window"`
	ObservedFrom string `json:"observedFrom"`
	Control      struct {
		Passed bool `json:"passed"`
	} `json:"control"`
}

func collect(t *testing.T, layer *Layer, call engine.Call) (collectAnswer, engine.ToolResult, string) {
	t.Helper()
	result, err := layer.Invoke(context.Background(), call)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		return collectAnswer{}, result, ""
	}
	raw, err := layer.content.Get(context.Background(), result.ResultRef)
	if err != nil {
		t.Fatal(err)
	}
	var answer collectAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return answer, result, string(raw)
}

func TestCollect_offsetEndsOnAShortPage_isComplete(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 5, controls: 1}
	answer, result, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Coverage != EvidenceComplete || answer.Events != 5 {
		t.Fatalf("answer = %+v, want 5 events complete", answer)
	}
	if !result.Labels.HasAny(domain.LabelUntrusted) {
		t.Fatal("source content must be labeled untrusted")
	}
}

func TestCollect_cursorEndsWithoutANext_isComplete(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Path != "/events/page" {
			return nil, nil, false
		}
		if r.Query.Get("cursor") == "" {
			return []byte(`{"ok":1,"result":[{"_id":"a"},{"_id":"b"}],"pagination":{"nextCursor":"n1"}}`), nil, true
		}
		return []byte(`{"ok":1,"result":[{"_id":"c"}],"pagination":{"nextCursor":null}}`), nil, true
	}
	layer := evidenceLayer(t, source, func(i *Instance) {
		i.Evidence.Paging = EvidencePaging{
			Kind: EvidencePagingCursor, LimitParam: "limit", PageSize: 2,
			CursorParam: "cursor", NextCursor: "pagination.nextCursor",
		}
	})
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	if answer.Coverage != EvidenceComplete || answer.Events != 3 {
		t.Fatalf("answer = %+v, want 3 events complete", answer)
	}
}

// A source can refuse a query with a 200. Read as rows it is zero events, which
// is the one answer an investigation must never receive by accident.
func TestCollect_refusalInsideA200_failsInsteadOfCountingZero(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Path == "/events/page" {
			return []byte(`{"ok":0,"result":[],"error":"bad syntax"}`), nil, true
		}
		return nil, nil, false
	}
	_, result, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if !result.Failed || result.ErrorCode != CodeConnectorQueryRefused {
		t.Fatalf("result = %+v, want a refused query", result)
	}
}

func TestCollect_sourceDeniesAccess_failsInsteadOfCountingZero(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{}
	source.hook = func(EvidenceRequest) ([]byte, error, bool) {
		return nil, evidenceRemoteError{status: 403}, true
	}
	_, result, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if !result.Failed || result.ErrorCode != CodeConnectorUpstreamAuth {
		t.Fatalf("result = %+v, want the auth failure", result)
	}
}

// A body with no rows where rows belong means the source's shape changed.
// Reading that as an empty set would report absence from a parsing failure.
func TestCollect_rowsMissingFromTheBody_failsInsteadOfCountingZero(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Path == "/events/page" {
			return []byte(`{"ok":1,"items":[{"_id":"a"}]}`), nil, true
		}
		return nil, nil, false
	}
	_, result, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if !result.Failed || result.ErrorCode != CodeConnectorUpstreamFailed {
		t.Fatalf("result = %+v, want an upstream failure", result)
	}
}

// The control is executed under an approval too, and its answer decides whether
// absence can be concluded, so changing it must ask again.
func TestApprovalBinding_changesWithTheControlDeclaration(t *testing.T) {
	t.Parallel()
	layer := evidenceLayer(t, &fakeEvidenceSource{})
	call := collectCall(byUserArgs)
	before := layer.ApprovalBinding(call)
	changed := validEvidenceInstance()
	changed.Evidence.Queries[0].Path = "/events/elsewhere"
	if err := layer.SetInstances([]Instance{changed}); err != nil {
		t.Fatal(err)
	}
	if layer.ApprovalBinding(call) == before {
		t.Fatal("the binding ignored a changed control")
	}
}

// An offset the source ignores returns the first page forever. The collection
// must stop, and it must not call that complete.
func TestCollect_offsetStopsAdvancing_isAFloor(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Path == "/events/page" {
			return []byte(`{"ok":1,"result":[{"_id":"a"},{"_id":"b"}]}`), nil, true
		}
		return nil, nil, false
	}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Coverage != EvidenceFloor || answer.Reason != EvidenceReasonStalled || answer.Events != 2 {
		t.Fatalf("answer = %+v, want a floor of 2 for a stalled offset", answer)
	}
}

// Reordered, so the body differs and only the ids show nothing new arrived.
func TestCollect_pageHoldsOnlyEventsAlreadySeen_isAFloor(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Path != "/events/page" {
			return nil, nil, false
		}
		if r.Query.Get("offset") == "" {
			return []byte(`{"ok":1,"result":[{"_id":"a"},{"_id":"b"}]}`), nil, true
		}
		return []byte(`{"ok":1,"result":[{"_id":"b"},{"_id":"a"}]}`), nil, true
	}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Coverage != EvidenceFloor || answer.Reason != EvidenceReasonStalled || answer.Events != 2 {
		t.Fatalf("answer = %+v, want a floor of 2", answer)
	}
}

// The ids seen die with a call, so across a resume only the last page's digest
// can show the source is repeating itself.
func TestCollect_sourceRepeatsItsLastPageAfterAResume_isAFloor(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: evidencePagesPerCall * 2, controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Path != "/events/page" || r.Query.Get("offset") != strconv.Itoa(evidencePagesPerCall) {
			return nil, nil, false
		}
		page, err := source.page(EvidenceRequest{Query: map[string][]string{
			"offset": {strconv.Itoa(evidencePagesPerCall - 1)}, "limit": {"1"},
		}})
		return page, err, true
	}
	layer := evidenceLayer(t, source, func(i *Instance) { i.Evidence.Paging.PageSize = 1 })
	first, _, _ := collect(t, layer, collectCall(byUserArgs))
	if first.Coverage != EvidencePartial {
		t.Fatalf("first = %+v, want partial", first)
	}
	resume := collectCall(fmt.Sprintf(`{"resume":%q}`, first.Evidence))
	resume.Seq = 6
	last, _, _ := collect(t, layer, resume)
	if last.Coverage != EvidenceFloor || last.Reason != EvidenceReasonStalled || last.Events != first.Events {
		t.Fatalf("last = %+v, want a floor of %d", last, first.Events)
	}
}

func TestCollect_ceilingReached_isAFloor(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 50, controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) { i.Evidence.MaxEvents = 3 })
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	if answer.Coverage != EvidenceFloor || answer.Reason != EvidenceReasonCeiling || answer.Events != 3 {
		t.Fatalf("answer = %+v, want a floor of 3 at the ceiling", answer)
	}
}

// Exactly the ceiling on a final short page is the whole set, not a floor.
func TestCollect_lastEventLandsOnTheCeiling_isComplete(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 3, controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) { i.Evidence.MaxEvents = 3 })
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	if answer.Coverage != EvidenceComplete {
		t.Fatalf("answer = %+v, want complete", answer)
	}
}

func TestCollect_pageBudgetRunsOut_isPartialAndResumesToComplete(t *testing.T) {
	t.Parallel()
	total := evidencePagesPerCall*2 + 1
	source := &fakeEvidenceSource{events: total, controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) {
		i.Evidence.Paging.PageSize = 1
		i.Evidence.MaxEvents = total * 2
	})
	first, _, _ := collect(t, layer, collectCall(byUserArgs))
	if first.Coverage != EvidencePartial || first.Reason != EvidenceReasonPageBudget {
		t.Fatalf("first = %+v, want partial on the page budget", first)
	}
	resume := collectCall(fmt.Sprintf(`{"resume":%q}`, first.Evidence))
	resume.Seq = 6
	second, _, _ := collect(t, layer, resume)
	if second.Coverage != EvidencePartial {
		t.Fatalf("second = %+v, want still partial", second)
	}
	resume = collectCall(fmt.Sprintf(`{"resume":%q}`, second.Evidence))
	resume.Seq = 8
	last, _, _ := collect(t, layer, resume)
	if last.Coverage != EvidenceComplete || last.Events != int64(total) {
		t.Fatalf("last = %+v, want %d events complete", last, total)
	}
}

// The source failing after pages are held must not lose them or call them whole.
func TestCollect_sourceFailsAfterSomePages_keepsThemAsPartial(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 9, controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		if r.Query.Get("offset") == "2" {
			return nil, evidenceRemoteError{status: 503}, true
		}
		return nil, nil, false
	}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Coverage != EvidencePartial || answer.Reason != EvidenceReasonInterrupted || answer.Events != 2 {
		t.Fatalf("answer = %+v, want 2 events partial", answer)
	}
}

// The caller's cancellation is not the source's failure: the engine retries it.
func TestCollect_callerCancelled_returnsTheContextError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	source := &fakeEvidenceSource{}
	source.hook = func(EvidenceRequest) ([]byte, error, bool) {
		cancel()
		return nil, context.Canceled, true
	}
	_, err := evidenceLayer(t, source).Invoke(ctx, collectCall(byUserArgs))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// The value the bug accepts: the pattern `.*` passes a quote, so only the
// platform's own floor stands between a model and the filter's structure.
func TestCollect_valueCarriesAQuote_neverReachesTheSource(t *testing.T) {
	t.Parallel()
	for _, value := range []string{`a' or user ne 'x`, `a" or 1`, `a\`, "a\nb", "`a`"} {
		source := &fakeEvidenceSource{controls: 1}
		args, _ := json.Marshal(map[string]any{
			"query": "sloppy", "params": map[string]string{"user": value},
			"window": map[string]int{"periodSeconds": 3600},
		})
		_, result, _ := collect(t, evidenceLayer(t, source), collectCall(string(args)))
		if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
			t.Fatalf("value %q: result = %+v, want bad arguments", value, result)
		}
		if len(source.requests) != 0 {
			t.Fatalf("value %q reached the source: %+v", value, source.requests)
		}
	}
}

func TestCollect_badArguments_neverReachTheSource(t *testing.T) {
	t.Parallel()
	for name, args := range map[string]string{
		"off the pattern":   `{"query":"by_user","params":{"user":"NOT-AN-EMAIL"},"window":{"periodSeconds":3600}}`,
		"pattern inside":    `{"query":"by_user","params":{"user":"NOT a@example.com ANYWAY"},"window":{"periodSeconds":3600}}`,
		"filter missing":    `{"query":"by_user","params":{},"window":{"periodSeconds":3600}}`,
		"empty value":       `{"query":"by_user","params":{"user":""},"window":{"periodSeconds":3600}}`,
		"undeclared param":  `{"query":"by_user","params":{"user":"a@example.com","x":"y"},"window":{"periodSeconds":3600}}`,
		"unknown query":     `{"query":"nope","params":{},"window":{"periodSeconds":3600}}`,
		"unknown field":     `{"query":"all","sql":"drop","window":{"periodSeconds":3600}}`,
		"no window":         `{"query":"by_user","params":{"user":"a@example.com"}}`,
		"period over limit": `{"query":"by_user","params":{"user":"a@example.com"},"window":{"periodSeconds":86401}}`,
		"range on a period source": `{"query":"by_user","params":{"user":"a@example.com"},` +
			`"window":{"from":"2026-01-01T00:00:00Z","to":"2026-01-02T00:00:00Z"}}`,
		"resume with a query": `{"resume":"run://run-1/1/aa","query":"all"}`,
	} {
		source := &fakeEvidenceSource{controls: 1}
		_, result, _ := collect(t, evidenceLayer(t, source), collectCall(args))
		if !result.Failed {
			t.Fatalf("%s: result = %+v, want a failure", name, result)
		}
		if len(source.requests) != 0 {
			t.Fatalf("%s reached the source: %+v", name, source.requests)
		}
	}
}

// Nothing broader is ever tried: an empty filter stays empty, because the next
// candidate in a fallback chain is somebody else's traffic.
func TestCollect_filterMatchesNothing_staysEmptyAndNeverBroadens(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 0, controls: 1}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Coverage != EvidenceComplete || answer.Events != 0 || !answer.Control.Passed {
		t.Fatalf("answer = %+v, want an empty complete set with a passing control", answer)
	}
	var pages []EvidenceRequest
	for _, request := range source.requests {
		if request.Path == "/events/page" {
			pages = append(pages, request)
		}
	}
	if len(pages) != 1 || pages[0].Query.Get("query") != "user eq 'a@example.com'" {
		t.Fatalf("requests = %+v, want exactly the declared filter once", pages)
	}
}

func TestCollect_controlFindsNothingInTheWindow_recordsItFailed(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 0, controls: 0}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Control.Passed {
		t.Fatalf("answer = %+v, want the control failed", answer)
	}
}

func TestCollect_controlRunsOverTheSameWindow(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 0, controls: 1}
	collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	control := source.requests[0]
	if control.Path != "/events/all" || control.Query.Get("timeperiod") != "3600" || control.Query.Get("limit") != "1" {
		t.Fatalf("control request = %+v, want one row over the collection's window", control)
	}
}

func evidenceBlocks(t *testing.T, layer *Layer, ref string) (evidenceState, string) {
	t.Helper()
	instance, _, _ := layer.native("evidence.gateway.collect")
	state, code := layer.openEvidence(context.Background(), collectCall(""), instance, ref)
	if code != "" {
		t.Fatalf("open %s: %s", ref, code)
	}
	var all bytes.Buffer
	for _, block := range state.Blocks {
		raw, err := layer.content.Get(context.Background(), block)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(raw)
	}
	return state, all.String()
}

func TestCollect_keepsOnlyDeclaredFields(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 3, controls: 1}
	layer := evidenceLayer(t, source)
	answer, _, raw := collect(t, layer, collectCall(byUserArgs))
	_, stored := evidenceBlocks(t, layer, answer.Evidence)
	if strings.Contains(stored, "CANARY-undeclared") {
		t.Fatalf("an undeclared field was stored: %s", stored)
	}
	if !strings.Contains(stored, `"client_bytes":10`) {
		t.Fatalf("a declared field was lost: %s", stored)
	}
	if strings.Contains(raw, "e-1") {
		t.Fatalf("events reached the answer the model reads: %s", raw)
	}
}

// Past one payload's limit the events must survive whole: the store truncates
// silently, and a truncated block would be a complete collection that lies.
func TestCollect_moreThanOnePayload_spansBlocksWithoutTruncating(t *testing.T) {
	t.Parallel()
	total := 300
	padding := strings.Repeat("x", 8000)
	source := &fakeEvidenceSource{events: total, controls: 1}
	source.row = func(i int) map[string]any {
		return map[string]any{"_id": fmt.Sprintf("e-%d", i), "user": padding}
	}
	layer := evidenceLayer(t, source, func(i *Instance) { i.Evidence.Paging.PageSize = 100 })
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	state, stored := evidenceBlocks(t, layer, answer.Evidence)
	if answer.Coverage != EvidenceComplete || len(state.Blocks) < 2 {
		t.Fatalf("answer = %+v over %d blocks, want complete over several", answer, len(state.Blocks))
	}
	if lines := strings.Count(stored, "\n"); lines != total {
		t.Fatalf("stored %d events, want %d", lines, total)
	}
	if strings.Contains(stored, "truncated") {
		t.Fatal("a block was truncated")
	}
}

// A store smaller than a block must fail the call, not hand back a prefix.
func TestCollect_storeWouldTruncateABlock_fails(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 5, controls: 1}
	source.row = func(i int) map[string]any {
		return map[string]any{"_id": fmt.Sprintf("e-%d", i), "user": strings.Repeat("x", 4000)}
	}
	layer := evidenceLayer(t, source)
	layer.content = engine.NewMemoryContent().WithLimit(1024)
	if _, err := layer.Invoke(context.Background(), collectCall(byUserArgs)); err == nil {
		t.Fatal("a truncated block was accepted")
	}
}

func TestCollect_resumeFromAnotherRun_isRefused(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 1, controls: 1}
	layer := evidenceLayer(t, source)
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	other := collectCall(fmt.Sprintf(`{"resume":%q}`, answer.Evidence))
	other.RunID = "run-2"
	_, result, _ := collect(t, layer, other)
	if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, want the run fence", result)
	}
}

// Another tool's content in the same run, shaped exactly like evidence and
// claiming to be complete. Only the seal tells it apart.
func TestCollect_forgedStateInTheSameRun_isRefused(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 1, controls: 1}
	layer := evidenceLayer(t, source)
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	genuine, err := layer.content.Get(context.Background(), answer.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	forged := bytes.Replace(genuine, []byte(`\"events\":1`), []byte(`\"events\":9`), 1)
	if bytes.Equal(forged, genuine) {
		forged = bytes.Replace(genuine, []byte(`"events":1`), []byte(`"events":9`), 1)
	}
	if bytes.Equal(forged, genuine) {
		t.Fatalf("the forgery did not change anything: %s", genuine)
	}
	ref, err := layer.content.Put(context.Background(), "run-1", 5, forged)
	if err != nil {
		t.Fatal(err)
	}
	resume := collectCall(fmt.Sprintf(`{"resume":%q}`, ref))
	_, result, _ := collect(t, layer, resume)
	if !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, want the seal to refuse a forgery", result)
	}
}

func TestCollect_resumeAfterTheQueryChanged_isRefused(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 9, controls: 1}
	layer := evidenceLayer(t, source)
	first, _, _ := collect(t, layer, collectCall(byUserArgs))
	if first.Evidence == "" {
		t.Fatal("no evidence")
	}
	changed := validEvidenceInstance()
	changed.Evidence.Queries[1].Fields = []string{"_id"}
	if err := layer.SetInstances([]Instance{changed}); err != nil {
		t.Fatal(err)
	}
	_, result, _ := collect(t, layer, collectCall(fmt.Sprintf(`{"resume":%q}`, first.Evidence)))
	if !result.Failed || result.ErrorCode != CodeConnectorContractChanged {
		t.Fatalf("result = %+v, want a changed contract", result)
	}
}

func TestCollect_periodSnapsUpAndReportsTheOneUsed(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	args := `{"query":"by_user","params":{"user":"a@example.com"},"window":{"periodSeconds":7200}}`
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(args))
	if answer.Window.PeriodSeconds != 86400 || source.requests[1].Query.Get("timeperiod") != "86400" {
		t.Fatalf("window = %+v, requests = %+v, want the next accepted period", answer.Window, source.requests)
	}
	if answer.Window.From != "2026-11-04T12:00:00Z" || answer.Window.To != "2026-11-05T12:00:00Z" {
		t.Fatalf("window = %+v, want it absolute, from the engine's clock", answer.Window)
	}
}

func TestCollect_rangeWindowSendsTheDeclaredFormat(t *testing.T) {
	t.Parallel()
	for format, want := range map[string]string{
		EvidenceFormatISO: "2026-01-01T00:00:00Z", EvidenceFormatEpoch: "1767225600",
	} {
		source := &fakeEvidenceSource{controls: 1}
		layer := evidenceLayer(t, source, func(i *Instance) {
			i.Evidence.Window = EvidenceWindow{
				Kind: EvidenceWindowRange, StartParam: "from", EndParam: "to", Format: format,
			}
		})
		args := `{"query":"by_user","params":{"user":"a@example.com"},` +
			`"window":{"from":"2026-01-01T00:00:00Z","to":"2026-01-02T00:00:00Z"}}`
		answer, _, _ := collect(t, layer, collectCall(args))
		if answer.Coverage != EvidenceComplete || source.requests[1].Query.Get("from") != want {
			t.Fatalf("%s: answer = %+v, requests = %+v", format, answer, source.requests)
		}
	}
}

func TestCollect_rangeRunsBackwards_isRefused(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) {
		i.Evidence.Window = EvidenceWindow{
			Kind: EvidenceWindowRange, StartParam: "from", EndParam: "to", Format: EvidenceFormatISO,
		}
	})
	args := `{"query":"by_user","params":{"user":"a@example.com"},` +
		`"window":{"from":"2026-01-02T00:00:00Z","to":"2026-01-01T00:00:00Z"}}`
	_, result, _ := collect(t, layer, collectCall(args))
	if !result.Failed || len(source.requests) != 0 {
		t.Fatalf("result = %+v, requests = %+v, want a refusal", result, source.requests)
	}
}

func TestCollect_windowlessQuery_reportsThatItHasNoWindow(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) {
		i.Evidence.Queries[1].Windowless = true
	})
	args := `{"query":"by_user","params":{"user":"a@example.com"}}`
	answer, _, _ := collect(t, layer, collectCall(args))
	page := source.requests[len(source.requests)-1]
	if answer.Window.Kind != EvidenceWindowNone || page.Query.Has("timeperiod") {
		t.Fatalf("answer = %+v, request = %+v, want no window", answer, page)
	}
}

func TestCollect_countsEventsTheSourceCouldNotMeasure(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 4, controls: 1}
	source.row = func(i int) map[string]any {
		row := map[string]any{"_id": fmt.Sprintf("e-%d", i)}
		if i%2 == 0 {
			row["bypass_traffic"] = "yes"
		}
		return row
	}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.Unmeasured != 2 {
		t.Fatalf("answer = %+v, want 2 unmeasured", answer)
	}
}

func TestCollect_reportsWhatTheEventsActuallyCover(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 3, controls: 1}
	answer, _, _ := collect(t, evidenceLayer(t, source), collectCall(byUserArgs))
	if answer.ObservedFrom != "2026-09-21T14:13:20Z" {
		t.Fatalf("answer = %+v, want the earliest event's time", answer)
	}
}

func TestCollect_tokenReachesTheSourceAndNothingElse(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: 1, controls: 1}
	layer := evidenceLayer(t, source)
	answer, _, raw := collect(t, layer, collectCall(byUserArgs))
	if source.seen[0] != "CANARY-token" {
		t.Fatalf("credential = %q", source.seen[0])
	}
	state, err := layer.content.Get(context.Background(), answer.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "CANARY-token") || strings.Contains(string(state), "CANARY-token") {
		t.Fatal("the token was stored")
	}
}

func TestCollect_withoutARuntime_isUnavailable(t *testing.T) {
	t.Parallel()
	layer := New(nil, nil, engine.NewMemoryContent(), nil)
	if err := layer.SetInstances([]Instance{validEvidenceInstance()}); err != nil {
		t.Fatal(err)
	}
	result, err := layer.Invoke(context.Background(), collectCall(byUserArgs))
	if err != nil || !result.Failed || result.ErrorCode != CodeConnectorUnavailable {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestCheck_reportsTheCredentialAndTheControl(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	layer := evidenceLayer(t, source)
	call := collectCall(`{}`)
	call.Tool = "evidence.gateway.check"
	result, err := layer.Invoke(context.Background(), call)
	if err != nil || result.Failed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	raw, _ := layer.content.Get(context.Background(), result.ResultRef)
	var answer struct {
		Control evidenceControl `json:"control"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || !answer.Control.Passed {
		t.Fatalf("answer = %s, err = %v, want the control passed", raw, err)
	}
	if source.requests[0].Query.Get("timeperiod") != "3600" {
		t.Fatalf("request = %+v, want the shortest declared period", source.requests[0])
	}
}

func TestCheck_rangeSource_asksOverAnAbsoluteWindow(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) {
		i.Evidence.Window = EvidenceWindow{
			Kind: EvidenceWindowRange, StartParam: "from", EndParam: "to", Format: EvidenceFormatISO,
		}
	})
	call := collectCall(`{}`)
	call.Tool = "evidence.gateway.check"
	if result, err := layer.Invoke(context.Background(), call); err != nil || result.Failed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	request := source.requests[0]
	if request.Query.Get("from") != "2026-11-04T12:00:00Z" || request.Query.Get("to") != "2026-11-05T12:00:00Z" {
		t.Fatalf("request = %+v", request)
	}
}

func TestCheck_credentialRefused_fails(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{}
	source.hook = func(EvidenceRequest) ([]byte, error, bool) {
		return nil, evidenceRemoteError{status: 401}, true
	}
	call := collectCall(`{}`)
	call.Tool = "evidence.gateway.check"
	result, err := evidenceLayer(t, source).Invoke(context.Background(), call)
	if err != nil || !result.Failed || result.ErrorCode != CodeConnectorUpstreamAuth {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestSchema_collectOffersTheDeclaredQueries(t *testing.T) {
	t.Parallel()
	layer := evidenceLayer(t, &fakeEvidenceSource{})
	_, description, schema, ok := layer.Schema("evidence.gateway.collect")
	if !ok {
		t.Fatal("no schema")
	}
	requireProviderPropertyMap(t, "evidence.collect", schema, []string{"query", "params", "window", "resume"})
	for _, want := range []string{"by_user", "One user's events.", "user"} {
		if !strings.Contains(description, want) {
			t.Fatalf("description %q does not offer %q", description, want)
		}
	}
}

// Changing a query under the same id asks again instead of borrowing a decision.
func TestApprovalBinding_changesWithTheQueryDeclaration(t *testing.T) {
	t.Parallel()
	layer := evidenceLayer(t, &fakeEvidenceSource{})
	call := collectCall(byUserArgs)
	before := layer.ApprovalBinding(call)
	if before == "" {
		t.Fatal("no binding")
	}
	call.ContractDigest = before
	if err := layer.Reserve(context.Background(), call); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	changed := validEvidenceInstance()
	changed.Evidence.Queries[1].Filters[0].Value = "user ne '{user}'"
	if err := layer.SetInstances([]Instance{changed}); err != nil {
		t.Fatal(err)
	}
	if layer.ApprovalBinding(call) == before {
		t.Fatal("the binding ignored a changed filter")
	}
	if err := layer.Reserve(context.Background(), call); !errors.Is(err, ErrEvidenceContractChanged) {
		t.Fatalf("Reserve = %v, want the contract refused", err)
	}
}
