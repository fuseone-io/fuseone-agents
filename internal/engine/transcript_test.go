package engine

import (
	"context"
	"encoding/json"
	"fmt"
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
	body := []byte(strings.Repeat("document ", 6<<10))
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

	if len(turns) != 1 || string(turns[0].Content) != string(body) {
		t.Fatalf("turns = %+v, want the non-compactable result untouched", turns)
	}
}

func TestBuildTranscript_largeGitHubIssueResult_staysWhole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryContent()
	body := []byte(strings.Repeat("issue body ", 6<<10))
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
