package engine

import (
	"context"
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

type inlineContent string

func (c inlineContent) Get(context.Context, string) ([]byte, error) { return []byte(c), nil }

func (inlineContent) Put(context.Context, domain.RunID, int64, []byte) (string, error) {
	return "", nil
}
