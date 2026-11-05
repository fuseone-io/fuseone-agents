package connectortools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

type analyzeAnswer struct {
	Events     int64  `json:"events"`
	Duplicates int64  `json:"duplicates"`
	Outside    int64  `json:"outsideWindow"`
	Undated    int64  `json:"undated"`
	Unmeasured int64  `json:"unmeasured"`
	Coverage   string `json:"coverage"`
	Reason     string `json:"reason"`
	Outcome    string `json:"outcome"`
	Sums       map[string]struct {
		Total   float64 `json:"total"`
		Missing int64   `json:"missing"`
		Zero    int64   `json:"zero"`
	} `json:"sums"`
	Groups []struct {
		Field    string `json:"field"`
		Distinct int    `json:"distinct"`
		Values   []struct {
			Value string             `json:"value"`
			Count int64              `json:"count"`
			Sums  map[string]float64 `json:"sums"`
		} `json:"values"`
	} `json:"groups"`
}

func analyzeCall(args string) engine.Call {
	call := collectCall(args)
	call.Tool = "evidence.gateway.analyze"
	call.Seq = 8
	return call
}

func analyze(t *testing.T, layer *Layer, args string) (analyzeAnswer, engine.ToolResult) {
	t.Helper()
	result, err := layer.Invoke(context.Background(), analyzeCall(args))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Failed {
		return analyzeAnswer{}, result
	}
	raw, err := layer.content.Get(context.Background(), result.ResultRef)
	if err != nil {
		t.Fatal(err)
	}
	var answer analyzeAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return answer, result
}

func trafficSource(rows ...map[string]any) *fakeEvidenceSource {
	source := &fakeEvidenceSource{events: len(rows), controls: 1}
	source.row = func(i int) map[string]any { return rows[i] }
	return source
}

func collected(t *testing.T, source *fakeEvidenceSource, edit ...func(*Instance)) (*Layer, string) {
	t.Helper()
	all := append([]func(*Instance){func(i *Instance) {
		i.Evidence.Queries[1].Fields = []string{
			"_id", "timestamp", "user", "domain", "client_bytes", "bypass_traffic",
		}
	}}, edit...)
	layer := evidenceLayer(t, source, all...)
	answer, _, _ := collect(t, layer, collectCall(byUserArgs))
	if answer.Evidence == "" {
		t.Fatal("nothing collected")
	}
	return layer, answer.Evidence
}

func event(id string, at int, domain string, up any) map[string]any {
	return map[string]any{"_id": id, "timestamp": at, "user": "a@example.com", "domain": domain, "client_bytes": up}
}

func TestAnalyze_sumsAndGroupsWithTheCoverageInherited(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(
		event("1", 100, "drive.example.com", 500),
		event("2", 200, "drive.example.com", 100),
		event("3", 300, "chat.example.org", 7),
	))
	answer, result := analyze(t, layer, fmt.Sprintf(
		`{"evidence":%q,"groupBy":["domain"],"sum":["client_bytes"]}`, ref))
	if answer.Events != 3 || answer.Coverage != EvidenceComplete || answer.Sums["client_bytes"].Total != 607 {
		t.Fatalf("answer = %+v", answer)
	}
	top := answer.Groups[0].Values[0]
	if top.Value != "drive.example.com" || top.Count != 2 || top.Sums["client_bytes"] != 600 {
		t.Fatalf("top group = %+v, want the biggest first with its sum", top)
	}
	if !result.Labels.HasAny("untrusted") {
		t.Fatal("aggregates of source content stay untrusted")
	}
}

// A sum over a field that came back zero or missing is not a measurement of
// zero: some event types never carry it, and the reader has to see that.
func TestAnalyze_reportsWhatASumCouldNotCount(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(
		event("1", 100, "a.example.com", 0),
		event("2", 200, "a.example.com", "not-a-number"),
		map[string]any{"_id": "3", "timestamp": 300},
	))
	answer, _ := analyze(t, layer, fmt.Sprintf(`{"evidence":%q,"sum":["client_bytes"]}`, ref))
	sum := answer.Sums["client_bytes"]
	if sum.Total != 0 || sum.Zero != 1 || sum.Missing != 2 {
		t.Fatalf("sum = %+v, want one zero and two uncounted", sum)
	}
}

func TestAnalyze_windowCutsAndSaysWhatItLeftOut(t *testing.T) {
	t.Parallel()
	inside := 1790000000
	source := trafficSource(
		event("1", inside, "a.example.com", 1),
		event("2", inside+7200, "a.example.com", 1),
		map[string]any{"_id": "3"},
	)
	layer := evidenceLayer(t, source, func(i *Instance) {
		i.Evidence.Window = EvidenceWindow{
			Kind: EvidenceWindowRange, StartParam: "from", EndParam: "to", Format: EvidenceFormatEpoch,
		}
	})
	first, _, _ := collect(t, layer, collectCall(`{"query":"by_user","params":{"user":"a@example.com"},`+
		`"window":{"from":"2026-09-01T00:00:00Z","to":"2026-10-01T00:00:00Z"}}`))
	answer, _ := analyze(t, layer, fmt.Sprintf(
		`{"evidence":%q,"window":{"from":"2026-09-21T14:13:20Z","to":"2026-09-21T15:13:20Z"}}`, first.Evidence))
	if answer.Events != 1 || answer.Outside != 1 || answer.Undated != 1 {
		t.Fatalf("answer = %+v, want 1 inside, 1 outside, 1 undated", answer)
	}
}

// Asking about a time nobody collected must not answer as if it had been looked at.
func TestAnalyze_windowBeyondTheCollection_isRefused(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(event("1", 100, "a.example.com", 1)))
	_, result := analyze(t, layer, fmt.Sprintf(
		`{"evidence":%q,"window":{"from":"2020-01-01T00:00:00Z","to":"2020-01-02T00:00:00Z"}}`, ref))
	if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
		t.Fatalf("result = %+v, want a refusal", result)
	}
}

func TestAnalyze_windowOverAQueryWithNoTimeField_isRefused(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(event("1", 100, "a.example.com", 1)), func(i *Instance) {
		i.Evidence.Queries[1].TimeField = ""
	})
	_, result := analyze(t, layer, fmt.Sprintf(
		`{"evidence":%q,"window":{"from":"2026-11-05T11:30:00Z","to":"2026-11-05T11:40:00Z"}}`, ref))
	if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
		t.Fatalf("result = %+v, want a refusal", result)
	}
}

func TestAnalyze_refusesFieldsTheQueryDoesNotKeep(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(event("1", 100, "a.example.com", 1)))
	for _, args := range []string{
		`{"evidence":%q,"groupBy":["secret_field"]}`,
		`{"evidence":%q,"sum":["secret_field"]}`,
		`{"evidence":%q,"match":{"field":"secret_field","equals":"x"}}`,
		`{"evidence":%q,"match":{"field":"domain"}}`,
		`{"evidence":%q,"match":{"field":"domain","equals":"x","suffix":"y"}}`,
		`{"evidence":%q,"top":1000}`,
		`{"evidence":%q,"sql":"x"}`,
	} {
		_, result := analyze(t, layer, fmt.Sprintf(args, ref))
		if !result.Failed || result.ErrorCode != CodeConnectorBadArguments {
			t.Fatalf("args %s: result = %+v, want bad arguments", args, result)
		}
	}
}

// A relative window moves between resumes, so the same event can be held twice.
func TestAnalyze_countsAnEventOnce(t *testing.T) {
	t.Parallel()
	total := evidencePagesPerCall + 1
	source := &fakeEvidenceSource{events: total, controls: 1}
	source.hook = func(r EvidenceRequest) ([]byte, error, bool) {
		// The window moved by one between calls: the page after the resume
		// starts one event earlier than the offset says.
		if r.Path == "/events/page" && r.Query.Get("offset") == fmt.Sprint(evidencePagesPerCall) {
			return []byte(`{"ok":1,"result":[{"_id":"e-49"},{"_id":"e-50"}]}`), nil, true
		}
		return nil, nil, false
	}
	layer := evidenceLayer(t, source, func(i *Instance) { i.Evidence.Paging.PageSize = 1 })
	first, _, _ := collect(t, layer, collectCall(byUserArgs))
	resume := collectCall(fmt.Sprintf(`{"resume":%q}`, first.Evidence))
	resume.Seq = 6
	second, _, _ := collect(t, layer, resume)
	answer, _ := analyze(t, layer, fmt.Sprintf(`{"evidence":%q}`, second.Evidence))
	if answer.Events != int64(total) || answer.Duplicates != 1 {
		t.Fatalf("answer = %+v, second = %+v, want %d events and one duplicate", answer, second, total)
	}
}

func TestAnalyze_suffixMatchesOnALabelBoundary(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(
		event("1", 100, "example.com", 1),
		event("2", 100, "Files.Example.com", 1),
		event("3", 100, "notexample.com", 1),
		event("4", 100, "example.com.evil.test", 1),
	))
	answer, _ := analyze(t, layer, fmt.Sprintf(
		`{"evidence":%q,"match":{"field":"domain","suffix":"example.com"}}`, ref))
	if answer.Events != 2 || answer.Outcome != EvidenceObserved {
		t.Fatalf("answer = %+v, want only the two real subdomains", answer)
	}
}

func TestAnalyze_outcomeSaysOnlyWhatTheCoverageSupports(t *testing.T) {
	t.Parallel()
	noMatch := `{"evidence":%q,"match":{"field":"domain","equals":"nowhere.example.com"}}`
	row := event("1", 100, "a.example.com", 1)
	unmeasured := event("2", 100, "b.example.com", 0)
	unmeasured["bypass_traffic"] = "yes"
	for name, test := range map[string]struct {
		source *fakeEvidenceSource
		edit   func(*Instance)
		args   string
		want   string
	}{
		"a match":          {source: trafficSource(row), args: `{"evidence":%q}`, want: EvidenceObserved},
		"complete, absent": {source: trafficSource(row), args: noMatch, want: EvidenceNotObserved},
		"control failed": {
			source: func() *fakeEvidenceSource { s := trafficSource(row); s.controls = 0; return s }(),
			args:   noMatch, want: EvidenceNotObservable,
		},
		"a floor": {
			source: trafficSource(row, event("2", 100, "a.example.com", 1), event("3", 100, "a.example.com", 1)),
			edit:   func(i *Instance) { i.Evidence.MaxEvents = 1 },
			args:   noMatch, want: EvidenceNotObservable,
		},
		"events the source could not measure": {
			source: trafficSource(row, unmeasured), args: noMatch, want: EvidenceNotObservable,
		},
	} {
		edits := []func(*Instance){}
		if test.edit != nil {
			edits = append(edits, test.edit)
		}
		layer, ref := collected(t, test.source, edits...)
		answer, _ := analyze(t, layer, fmt.Sprintf(test.args, ref))
		if answer.Outcome != test.want {
			t.Errorf("%s: outcome = %q, want %q (%+v)", name, answer.Outcome, test.want, answer)
		}
	}
}

func TestAnalyze_partialCollection_cannotConcludeAbsence(t *testing.T) {
	t.Parallel()
	source := &fakeEvidenceSource{events: evidencePagesPerCall + 5, controls: 1}
	layer := evidenceLayer(t, source, func(i *Instance) { i.Evidence.Paging.PageSize = 1 })
	first, _, _ := collect(t, layer, collectCall(byUserArgs))
	answer, _ := analyze(t, layer, fmt.Sprintf(
		`{"evidence":%q,"match":{"field":"user","equals":"nobody@example.com"}}`, first.Evidence))
	if answer.Outcome != EvidenceNotObservable || answer.Coverage != EvidencePartial {
		t.Fatalf("answer = %+v", answer)
	}
}

func TestAnalyze_topBoundsWhatComesBackAndCountsTheRest(t *testing.T) {
	t.Parallel()
	rows := []map[string]any{}
	for i := range 5 {
		rows = append(rows, event(fmt.Sprint(i), 100, fmt.Sprintf("d%d.example.com", i), 1))
	}
	layer, ref := collected(t, trafficSource(rows...))
	answer, _ := analyze(t, layer, fmt.Sprintf(`{"evidence":%q,"groupBy":["domain"],"top":2}`, ref))
	if len(answer.Groups[0].Values) != 2 || answer.Groups[0].Distinct != 5 {
		t.Fatalf("groups = %+v, want 2 of 5 distinct", answer.Groups)
	}
}

// An untrusted value can be any length, and it goes to the model.
func TestAnalyze_boundsAGroupValue(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(event("1", 100, strings.Repeat("x", 5000), 1)))
	answer, _ := analyze(t, layer, fmt.Sprintf(`{"evidence":%q,"groupBy":["domain"]}`, ref))
	if got := len(answer.Groups[0].Values[0].Value); got > maxEvidenceGroupValue {
		t.Fatalf("value length = %d", got)
	}
}

func TestAnalyze_evidenceFromAnotherRun_isRefused(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(event("1", 100, "a.example.com", 1)))
	call := analyzeCall(fmt.Sprintf(`{"evidence":%q}`, ref))
	call.RunID = "run-2"
	result, err := layer.Invoke(context.Background(), call)
	if err != nil || !result.Failed || result.ErrorCode != CodeConnectorGuardRefused {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

// erasingContent has lost one block, as erasure of a subject's data can do.
type erasingContent struct {
	engine.ContentStore
	erased string
}

func (e erasingContent) Get(ctx context.Context, ref string) ([]byte, error) {
	if ref == e.erased {
		return nil, domain.ErrContentErased
	}
	return e.ContentStore.Get(ctx, ref)
}

func TestAnalyze_aBlockWasErased_failsRatherThanCountingLess(t *testing.T) {
	t.Parallel()
	layer, ref := collected(t, trafficSource(event("1", 100, "a.example.com", 1)))
	state, _ := evidenceBlocks(t, layer, ref)
	layer.content = erasingContent{ContentStore: layer.content, erased: state.Blocks[0]}
	_, result := analyze(t, layer, fmt.Sprintf(`{"evidence":%q}`, ref))
	if !result.Failed || result.ErrorCode != CodeConnectorArtifactMissing {
		t.Fatalf("result = %+v, want the missing evidence named", result)
	}
}

func TestSchema_analyzeOffersTheFieldsOfEachQuery(t *testing.T) {
	t.Parallel()
	layer := evidenceLayer(t, &fakeEvidenceSource{})
	_, description, schema, ok := layer.Schema("evidence.gateway.analyze")
	if !ok {
		t.Fatal("no schema")
	}
	requireProviderPropertyMap(t, "evidence.analyze", schema, []string{"evidence", "window", "groupBy", "sum", "match", "top"})
	if !strings.Contains(description, "by_user") || !strings.Contains(description, "client_bytes") {
		t.Fatalf("description = %q", description)
	}
}

// From the stored setting to the tool an agent sees, through collect and analyze:
// nothing here injects a config, so a field dropped in between fails.
func TestEvidenceJourney_fromTheSettingToAnAnswer(t *testing.T) {
	t.Parallel()
	stored, err := SettingValue(validEvidenceInstance())
	if err != nil {
		t.Fatal(err)
	}
	var row StoredInstance
	if err := json.Unmarshal(stored, &row); err != nil {
		t.Fatal(err)
	}
	instance := validEvidenceInstance()
	instance.Evidence = storedEvidence(row.Evidence)
	entries := toolEntriesFor([]Instance{instance})
	want := map[string]bool{}
	for _, entry := range entries {
		want[string(entry.ID)] = entry.Untrusted && entry.Native
	}
	for _, id := range []string{"evidence.gateway.check", "evidence.gateway.collect", "evidence.gateway.analyze"} {
		if !want[id] {
			t.Fatalf("entries = %+v, missing an untrusted native %s", entries, id)
		}
	}
	source := &fakeEvidenceSource{events: 3, controls: 1}
	layer := New(nil, nil, engine.NewMemoryContent(), nil).WithEvidence(source)
	if err := layer.SetInstances([]Instance{instance}); err != nil {
		t.Fatal(err)
	}
	first, _, _ := collect(t, layer, collectCall(byUserArgs))
	answer, _ := analyze(t, layer, fmt.Sprintf(`{"evidence":%q,"sum":["client_bytes"]}`, first.Evidence))
	if answer.Events != 3 || answer.Sums["client_bytes"].Total != 30 || answer.Outcome != EvidenceObserved {
		t.Fatalf("answer = %+v", answer)
	}
}
