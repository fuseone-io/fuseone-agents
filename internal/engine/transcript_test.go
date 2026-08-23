package engine

import (
	"context"
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

func TestBuildTranscript_largeNonObservabilityResult_staysWhole(t *testing.T) {
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
		t.Fatalf("turns = %+v, want the non-observability result untouched", turns)
	}
}

type inlineContent string

func (c inlineContent) Get(context.Context, string) ([]byte, error) { return []byte(c), nil }

func (inlineContent) Put(context.Context, domain.RunID, int64, []byte) (string, error) {
	return "", nil
}
