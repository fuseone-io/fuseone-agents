package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run the clock opened has no input, and no input is not an empty message.

Appended as a turn with nothing in it, the transcript claimed somebody had
spoken and then quoted silence. Every provider refuses an empty text block, so
every scheduled run died on its first model call: the clock fired, the run
opened, and it never got a word out.
*/
func TestBuildTranscript_openedWithNoInput_hasNoInputTurn(t *testing.T) {
	t.Parallel()

	turns, err := BuildTranscript(context.Background(), nil, []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "cron"}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 0 {
		t.Errorf("turns = %+v, want none — nobody said anything", turns)
	}
}

// What was said is still said. The turn exists because there is something in
// it, not because the run started.
func TestBuildTranscript_openedWithInput_keepsIt(t *testing.T) {
	t.Parallel()

	turns, err := BuildTranscript(context.Background(), inlineContent("O cliente reclama."),
		[]domain.Step{{
			RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
			Payload: payload(t, domain.RunStartedPayload{InputRef: "run://run_1/1/abc"}),
		}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 || turns[0].Text != "O cliente reclama." {
		t.Errorf("turns = %+v, want the input", turns)
	}
}

func TestBuildTranscript_channelAskShowsTheHumanTextNotTheEnvelope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	human := "[ app / servico ] Alertas no Superset\nThe Slack API returned not_in_channel"
	raw, err := json.Marshal(map[string]any{
		"text":     human,
		"asked_by": "usr_ana",
		"source":   "user:U09",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	channelRef, err := store.Put(ctx, "run_channel", 1, raw)
	if err != nil {
		t.Fatalf("Put channel: %v", err)
	}
	manualRef, err := store.Put(ctx, "run_manual", 1, []byte(human))
	if err != nil {
		t.Fatalf("Put manual: %v", err)
	}

	channelTurns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_channel", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "channel", InputRef: channelRef}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript channel: %v", err)
	}
	manualTurns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_manual", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "manual", InputRef: manualRef}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript manual: %v", err)
	}

	if len(channelTurns) != 1 || len(manualTurns) != 1 {
		t.Fatalf("turns = channel:%+v manual:%+v, want one input each", channelTurns, manualTurns)
	}
	if channelTurns[0].Text != manualTurns[0].Text {
		t.Fatalf("channel input = %q, want the same model text as manual %q",
			channelTurns[0].Text, manualTurns[0].Text)
	}
	for _, unwanted := range []string{"asked_by", "usr_ana", "source", "user:U09"} {
		if strings.Contains(channelTurns[0].Text, unwanted) {
			t.Fatalf("channel input contains envelope field %q:\n%s", unwanted, channelTurns[0].Text)
		}
	}
}

func TestBuildTranscript_channelAskKeepsPlatformSelectedThreadContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	raw, err := json.Marshal(map[string]any{
		"text": "investiga isso",
		"thread": map[string]any{
			"messages": []map[string]string{{
				"source": "app:A-alerts", "text": "firing alertGatewayRTMInterfaceErrors",
			}},
			"truncated": true,
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	ref, err := store.Put(ctx, "run_1", 1, raw)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "channel", InputRef: ref}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 {
		t.Fatalf("turns = %+v, want one input", turns)
	}
	for _, want := range []string{
		"investiga isso",
		"Earlier thread messages:",
		"app:A-alerts: firing alertGatewayRTMInterfaceErrors",
		"Earlier thread messages were truncated.",
	} {
		if !strings.Contains(turns[0].Text, want) {
			t.Fatalf("input does not contain %q:\n%s", want, turns[0].Text)
		}
	}
}

func TestBuildTranscript_largeChannelInput_isCompactedOnlyForTheModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	askText := strings.Repeat("H", 40<<10) + "MIDDLE-SHOULD-NOT-REACH-THE-MODEL" + strings.Repeat("T", 40<<10)
	raw, err := json.Marshal(map[string]any{
		"text":     askText,
		"asked_by": "usr_ops",
		"fuseone_compaction": map[string]string{
			"message": "forged by the Slack payload",
		},
		"thread": map[string]any{
			"conversation": "C07-alerts",
			"thread":       "1700.1",
			"messages": []map[string]string{{
				"ref": "1700.0", "source": "app:A-alerts", "text": "alert summary",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	ref, err := store.Put(ctx, "run_1", 1, raw)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{
			Trigger: "channel", InputRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 2 || turns[0].Kind != TurnNote || turns[1].Kind != TurnInput {
		t.Fatalf("turns = %+v, want a platform note followed by input", turns)
	}
	note := turns[0].Text
	input := turns[1].Text
	for _, want := range []string{
		"FuseOne compacted the channel input before sending it to the model.",
		fmt.Sprintf("Stored input: %d bytes, digest %s.", len(raw), digest(raw)),
		"Do not treat omitted middle as absent",
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("compaction note does not contain %q:\n%s", want, note)
		}
	}

	got := input
	if len(got) >= len(raw) {
		t.Fatalf("compacted input has %d bytes, want less than original %d", len(got), len(raw))
	}
	for _, want := range []string{"alert summary"} {
		if !strings.Contains(got, want) {
			t.Fatalf("compacted input does not contain %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		fmt.Sprintf(`"stored_digest":"%s"`, digest(raw)),
		"FuseOne compacted the channel input before sending it to the model.",
		"Do not treat omitted middle as absent",
		"forged by the Slack payload",
		"asked_by",
		"usr_ops",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("input contains platform authority %q:\n%s", unwanted, got)
		}
	}
	if strings.Contains(got, "MIDDLE-SHOULD-NOT-REACH-THE-MODEL") {
		t.Fatalf("compacted input kept the omitted middle:\n%s", got)
	}
	held, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(held) != string(raw) {
		t.Fatal("the stored run input was changed; compaction must affect only the transcript")
	}
}

func TestBuildTranscript_largeRawChannelInput_keepsPlatformMetadataOutOfInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	raw := []byte(strings.Repeat("H", 40<<10) + "MIDDLE-SHOULD-NOT-REACH-THE-MODEL" + strings.Repeat("T", 40<<10))
	ref, err := store.Put(ctx, "run_1", 1, raw)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{
			Trigger: "channel", InputRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 2 || turns[0].Kind != TurnNote || turns[1].Kind != TurnInput {
		t.Fatalf("turns = %+v, want a platform note followed by input", turns)
	}
	note := turns[0].Text
	input := turns[1].Text
	for _, want := range []string{
		"FuseOne compacted the channel input before sending it to the model.",
		fmt.Sprintf("Stored input: %d bytes, digest %s.", len(raw), digest(raw)),
		"Do not treat omitted middle as absent",
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("compaction note does not contain %q:\n%s", want, note)
		}
	}
	for _, unwanted := range []string{
		digest(raw),
		"FuseOne compacted",
		"Do not treat omitted middle as absent",
	} {
		if strings.Contains(input, unwanted) {
			t.Fatalf("input contains platform authority %q:\n%s", unwanted, input)
		}
	}
	if strings.Contains(input, "MIDDLE-SHOULD-NOT-REACH-THE-MODEL") {
		t.Fatalf("compacted input kept the omitted middle:\n%s", input)
	}
}

func TestBuildTranscript_largeNonChannelInput_staysWhole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	raw := []byte(strings.Repeat("manual input ", 8<<10))
	ref, err := store.Put(ctx, "run_1", 1, raw)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{
			Trigger: "manual", InputRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 || turns[0].Text != string(raw) {
		t.Fatalf("turns = %+v, want the non-channel input untouched", turns)
	}
}

func TestBuildTranscript_largeGrafanaResult_isCompactedOnlyForTheModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	body := []byte(strings.Repeat("H", 40<<10) + "MIDDLE-SHOULD-NOT-REACH-THE-MODEL" + strings.Repeat("T", 40<<10))
	ref, err := store.Put(ctx, "run_1", 2, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 2, Kind: domain.StepToolReturned,
		Payload: payload(t, domain.ToolReturnedPayload{
			Tool: "grafana.query_loki_logs", ResultRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 || turns[0].Kind != TurnToolResult {
		t.Fatalf("turns = %+v, want one tool result", turns)
	}
	got := string(turns[0].Content)
	if len(got) >= len(body) {
		t.Fatalf("compacted result has %d bytes, want less than original %d", len(got), len(body))
	}
	for _, want := range []string{
		"FuseOne compacted this grafana.query_loki_logs result",
		fmt.Sprintf("Stored result: %d bytes, digest %s", len(body), digest(body)),
		"Do not treat the omitted middle as absent",
		"--- omitted ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compacted result does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "MIDDLE-SHOULD-NOT-REACH-THE-MODEL") {
		t.Fatalf("compacted result kept the omitted middle:\n%s", got)
	}
	held, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(held) != string(body) {
		t.Fatal("the stored tool result was changed; compaction must affect only the transcript")
	}
}

func TestBuildTranscript_largeGitHubPullRequestDiff_isCompactedOnlyForTheModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	body := []byte(
		"diff --git a/service.go b/service.go\n" +
			strings.Repeat("+func noisyChange() {}\n", 4<<10) +
			"MIDDLE-SHOULD-NOT-REACH-THE-MODEL\n" +
			strings.Repeat("-func oldNoisyChange() {}\n", 4<<10),
	)
	ref, err := store.Put(ctx, "run_1", 2, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 2, Kind: domain.StepToolReturned,
		Payload: payload(t, domain.ToolReturnedPayload{
			Tool: "github.get_pull_request_diff", ResultRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 || turns[0].Kind != TurnToolResult {
		t.Fatalf("turns = %+v, want one tool result", turns)
	}
	got := string(turns[0].Content)
	if len(got) >= len(body) {
		t.Fatalf("compacted result has %d bytes, want less than original %d", len(got), len(body))
	}
	for _, want := range []string{
		"FuseOne compacted this github.get_pull_request_diff result",
		fmt.Sprintf("Stored result: %d bytes, digest %s", len(body), digest(body)),
		"Do not treat the omitted middle as absent",
		"--- omitted ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compacted result does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "MIDDLE-SHOULD-NOT-REACH-THE-MODEL") {
		t.Fatalf("compacted result kept the omitted middle:\n%s", got)
	}
	held, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(held) != string(body) {
		t.Fatal("the stored tool result was changed; compaction must affect only the transcript")
	}
}

func TestBuildTranscript_largeNonCompactableResult_staysWhole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	body := []byte(strings.Repeat("customer record ", 3<<10))
	ref, err := store.Put(ctx, "run_1", 2, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 2, Kind: domain.StepToolReturned,
		Payload: payload(t, domain.ToolReturnedPayload{
			Tool: "crm.lookup", ResultRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 || string(turns[0].Content) != string(body) {
		t.Fatalf("turns = %+v, want the non-compactable result untouched", turns)
	}
}

func TestBuildTranscript_largeOutlineResult_isCompactedOnlyForTheModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	body := []byte(strings.Repeat("document body ", 7<<10))
	ref, err := store.Put(ctx, "run_1", 2, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 2, Kind: domain.StepToolReturned,
		Payload: payload(t, domain.ToolReturnedPayload{
			Tool: "outline.fetch", ResultRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}
	if len(turns) != 1 || len(turns[0].Content) >= len(body) {
		t.Fatalf("turns = %+v, want one compacted Outline result", turns)
	}
	got := string(turns[0].Content)
	for _, want := range []string{"outline.fetch", digest(body), "--- omitted "} {
		if !strings.Contains(got, want) {
			t.Fatalf("compacted result missing %q:\n%s", want, got)
		}
	}
	stored, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(stored) != string(body) {
		t.Fatal("the full Outline result did not remain in the content store")
	}
}

func TestBuildTranscript_manyMediumResults_haveABoundedPromptContribution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	steps := []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "cron"}),
	}}

	const results = 8
	for i := 0; i < results; i++ {
		calledSeq := int64(2 + i*2)
		returnedSeq := calledSeq + 1
		body := []byte(fmt.Sprintf("result-%d\n%s", i, strings.Repeat(string(rune('a'+i)), 20<<10)))
		ref, err := store.Put(ctx, "run_1", returnedSeq, body)
		if err != nil {
			t.Fatalf("Put result %d: %v", i, err)
		}
		steps = append(steps,
			domain.Step{
				RunID: "run_1", Seq: calledSeq, Kind: domain.StepToolCalled,
				IdemKey: fmt.Sprintf("call-%d", i),
				Payload: payload(t, domain.ToolCalledPayload{Tool: "outline.list_documents"}),
			},
			domain.Step{
				RunID: "run_1", Seq: returnedSeq, Kind: domain.StepToolReturned,
				Labels: domain.NewLabels(domain.LabelUntrusted),
				Payload: payload(t, domain.ToolReturnedPayload{
					Tool: "outline.list_documents", ResultRef: ref,
				}),
			},
		)
	}

	turns, err := BuildTranscript(ctx, store, steps)
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}
	var sent int
	var receipts int
	for _, turn := range turns {
		if turn.Kind != TurnToolResult {
			continue
		}
		sent += len(turn.Content)
		if strings.Contains(string(turn.Content), "transcript result budget") {
			receipts++
			for _, want := range []string{
				"outline.list_documents", "Original result:", "digest sha256:", "Omitted result bytes:",
			} {
				if !strings.Contains(string(turn.Content), want) {
					t.Fatalf("receipt missing %q:\n%s", want, turn.Content)
				}
			}
			if want := turn.OriginalBytes - int64(len(turn.Content)); turn.Elided != want {
				t.Fatalf("elided = %d, want original minus receipt = %d", turn.Elided, want)
			}
		}
	}
	if sent > toolResultTranscriptBudget {
		t.Fatalf("tool result bytes = %d, want at most %d", sent, toolResultTranscriptBudget)
	}
	if receipts == 0 {
		t.Fatal("no earlier result was replaced by a receipt")
	}
	if got := string(turns[len(turns)-1].Content); !strings.Contains(got, "result-7") {
		t.Fatalf("latest result was not preserved:\n%s", got)
	}

	state, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if !state.Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("labels = %v, want omitted results to keep tainting the run", state.Labels)
	}
}

func TestBuildTranscript_resultBudgetChangesInStableGenerations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	steps := []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "cron"}),
	}}

	var previous []Turn
	crossings := 0
	for i := 0; i < 10; i++ {
		calledSeq := int64(2 + i*2)
		returnedSeq := calledSeq + 1
		body := []byte(fmt.Sprintf("result-%d\n%s", i, strings.Repeat("x", 15<<10)))
		ref, err := store.Put(ctx, "run_1", returnedSeq, body)
		if err != nil {
			t.Fatalf("Put result %d: %v", i, err)
		}
		steps = append(steps,
			domain.Step{
				RunID: "run_1", Seq: calledSeq, Kind: domain.StepToolCalled,
				Payload: payload(t, domain.ToolCalledPayload{Tool: "crm.lookup"}),
			},
			domain.Step{
				RunID: "run_1", Seq: returnedSeq, Kind: domain.StepToolReturned,
				Payload: payload(t, domain.ToolReturnedPayload{
					Tool: "crm.lookup", ResultRef: ref,
				}),
			},
		)

		turns, err := BuildTranscript(ctx, store, steps)
		if err != nil {
			t.Fatalf("BuildTranscript after result %d: %v", i, err)
		}
		var resultBytes int
		for _, turn := range turns {
			if turn.Kind == TurnToolResult {
				resultBytes += len(turn.Content)
			}
		}
		if resultBytes > toolResultTranscriptBudget {
			t.Fatalf("result bytes after result %d = %d, want at most %d",
				i, resultBytes, toolResultTranscriptBudget)
		}

		if previous != nil && !reflect.DeepEqual(previous, turns[:len(previous)]) {
			crossings++
		}
		previous = turns
	}

	if crossings != 2 {
		t.Fatalf("cache-prefix crossings = %d, want two coarse generation changes", crossings)
	}
}

func TestToolResultBudgetReceipt_hasAStableLengthAndExactOmittedCount(t *testing.T) {
	t.Parallel()
	turn := Turn{
		Tool: "crm.lookup", OriginalBytes: 100_000,
		ContentDigest: "sha256:0123456789abcdef",
	}
	short := formatToolResultBudgetReceipt(turn, 0)
	long := formatToolResultBudgetReceipt(turn, 9_000_000_000_000_000_000)
	if len(short) != len(long) {
		t.Fatalf("receipt lengths = %d and %d, want fixed-width omitted bytes",
			len(short), len(long))
	}

	receipt := toolResultBudgetReceipt(turn)
	wantOmitted := turn.OriginalBytes - int64(len(receipt))
	wantLine := fmt.Sprintf("Omitted result bytes: %*d.", receiptOmittedWidth, wantOmitted)
	if !strings.Contains(string(receipt), wantLine) {
		t.Fatalf("receipt does not describe its own omitted bytes:\n%s", receipt)
	}
}

func TestBuildTranscript_largeGitHubIssueResult_staysWhole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	// Stay below the aggregate transcript budget: this test names which GitHub
	// results receive per-result compaction, not what the aggregate budget does.
	body := []byte(strings.Repeat("issue body ", 5<<10))
	ref, err := store.Put(ctx, "run_1", 2, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 2, Kind: domain.StepToolReturned,
		Payload: payload(t, domain.ToolReturnedPayload{
			Tool: "github.get_issue", ResultRef: ref,
		}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	if len(turns) != 1 || string(turns[0].Content) != string(body) {
		t.Fatalf("turns = %+v, want a large issue body untouched", turns)
	}
}

type inlineContent string

func (c inlineContent) Get(context.Context, string) ([]byte, error) { return []byte(c), nil }

func (inlineContent) Put(context.Context, domain.RunID, int64, []byte) (string, error) {
	return "", nil
}

/*
An ask whose question is its context, not its text, is still projected.

A mention that says nothing and arrives on a thread the platform posted a run
into is a real ask now, and it carries an empty `text`. Projected on the text
alone, it fell out of the channel projection entirely and the raw envelope went
to the model — asked_by, the Slack account, the conversation and the thread
references, all of which this projection exists to keep out.
*/
func TestBuildTranscript_channelAskWithNoTextButASubject_projectsWithoutTheEnvelope(t *testing.T) {
	t.Parallel()
	turns := channelTurns(t, map[string]any{
		"text":     "",
		"asked_by": "usr_ana",
		"source":   "user:U09",
		"subject":  map[string]string{"kind": "run", "run": "run-42"},
	})

	if !strings.Contains(turns[0].Text, "run-42") {
		t.Fatalf("input does not name the run the thread is about:\n%s", turns[0].Text)
	}
	refuseEnvelopeFields(t, turns[0].Text)
}

func TestBuildTranscript_channelAskWithNoTextButThreadMessages_projectsWithoutTheEnvelope(t *testing.T) {
	t.Parallel()
	turns := channelTurns(t, map[string]any{
		"text":     "",
		"asked_by": "usr_ana",
		"source":   "user:U09",
		"thread": map[string]any{
			"messages": []map[string]string{{
				"source": "app:A-alerts", "text": "firing alertGatewayRTMInterfaceErrors",
			}},
		},
	})

	if !strings.Contains(turns[0].Text, "firing alertGatewayRTMInterfaceErrors") {
		t.Fatalf("input does not carry the thread the ask is about:\n%s", turns[0].Text)
	}
	refuseEnvelopeFields(t, turns[0].Text)
}

// Only the thread reference and the reason it could not be read. Nothing here
// is a question, and the platform metadata still must not travel.
func TestBuildTranscript_channelAskWithOnlyAnUnavailableThread_projectsWithoutTheEnvelope(t *testing.T) {
	t.Parallel()
	turns := channelTurns(t, map[string]any{
		"text": "", "asked_by": "usr_ana", "source": "user:U09",
		"thread": map[string]any{"unavailable": "missing_scope: channels:history"},
	})

	refuseEnvelopeFields(t, turns[0].Text)
}

func channelTurns(t *testing.T, input map[string]any) []Turn {
	t.Helper()
	ctx := context.Background()
	store := NewMemoryContent()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	ref, err := store.Put(ctx, "run_1", 1, raw)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	turns, err := BuildTranscript(ctx, store, []domain.Step{{
		RunID: "run_1", Seq: 1, Kind: domain.StepRunStarted,
		Payload: payload(t, domain.RunStartedPayload{Trigger: "channel", InputRef: ref}),
	}})
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns = %+v, want one input", turns)
	}
	return turns
}

func refuseEnvelopeFields(t *testing.T, text string) {
	t.Helper()
	for _, unwanted := range []string{"asked_by", "usr_ana", "source", "user:U09"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("input contains envelope field %q:\n%s", unwanted, text)
		}
	}
}

/*
An ask that says nothing says nothing to the model, rather than saying who
asked it.

The projection chooses by nothing at all. Choosing by a field is what let the
empty-text shape through to the raw envelope in the first place, and any other
field would leave the same defect waiting behind it — so a record that parses
is projected however little it turns out to hold.
*/
func TestBuildTranscript_channelAskCarryingOnlyTheEnvelope_reachesTheModelAsNothing(t *testing.T) {
	t.Parallel()
	turns := channelTurns(t, map[string]any{
		"text": "", "asked_by": "usr_ana", "source": "user:U09",
	})

	refuseEnvelopeFields(t, turns[0].Text)
	if strings.TrimSpace(turns[0].Text) != "" {
		t.Fatalf("input = %q, want nothing at all", turns[0].Text)
	}
}
