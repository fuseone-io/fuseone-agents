package engine

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// The transcript is what the model sees: a projection of the ledger, rebuilt
// from scratch on every turn.
//
// Nothing is cached between turns, which is the point — a worker that picks up
// a half-finished run reconstructs the identical conversation from the ledger
// alone. Bulky content lives in the content store and is resolved here, so the
// ledger stays free of prompts, arguments and results (PRD AU-04).

// TurnKind is what a transcript entry represents.
type TurnKind uint8

const (
	// TurnInput is what started the run — the ticket, the alert, the message.
	TurnInput TurnKind = iota
	// TurnToolUse is a call the agent made, replayed so the model sees its own
	// earlier decisions.
	TurnToolUse
	// TurnToolResult is what came back, including failures.
	TurnToolResult
	// TurnNote is platform-authored context: a gate decision the model should
	// know about, an approval that landed.
	TurnNote
)

// Turn is one entry in the transcript.
type Turn struct {
	Kind TurnKind
	Text string

	// CallID pairs a TurnToolUse with its TurnToolResult. It is derived from
	// the run and sequence, so a rebuilt transcript pairs identically.
	CallID  string
	Tool    domain.ToolID
	Args    []byte
	Failed  bool
	Content []byte
	// Elided is what compaction removed before this turn was handed to the
	// model, in content bytes. Counted at the cut so the saving is something
	// the run measured rather than a subtraction across two records.
	Elided int64
	// OriginalBytes and ContentDigest describe the full result held in the
	// content store. They let the transcript replace an older result with an
	// honest receipt after per-result compaction has already happened.
	OriginalBytes int64
	ContentDigest string
}

// ContentStore holds payloads too large or too sensitive for the ledger.
//
// Declared here, by the consumer. The ledger records a reference and a digest;
// the bytes live here, under the installation's own retention rules.
type ContentStore interface {
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (ref string, err error)
	Get(ctx context.Context, ref string) ([]byte, error)
}

// BuildTranscript projects a run's steps into what the model should see.
//
// Steps that carry no meaning for the model — budget reservations, chain
// bookkeeping — are skipped. Everything the model needs to understand the
// state of its own work is included, in ledger order.
func BuildTranscript(ctx context.Context, store ContentStore, steps []domain.Step) ([]Turn, error) {
	turns := make([]Turn, 0, len(steps))

	for _, step := range steps {
		switch step.Kind {
		case domain.StepRunStarted:
			var p domain.RunStartedPayload
			if err := decode(step, &p); err != nil {
				return nil, err
			}
			text, err := resolve(ctx, store, p.InputRef)
			if err != nil {
				return nil, err
			}
			// A run the clock opened has no input, and no input is not an
			// empty message: a turn with nothing in it claims somebody spoke
			// and then quotes silence. Every provider refuses an empty text
			// block, so it is also how a scheduled run dies before its first
			// word.
			if len(text) > 0 {
				compacted, note := runInputForTranscript(p.Trigger, text)
				if note != "" {
					turns = append(turns, Turn{Kind: TurnNote, Text: note})
				}
				turns = append(turns, Turn{
					Kind: TurnInput,
					Text: string(compacted),
				})
			}

		case domain.StepToolCalled:
			var p domain.ToolCalledPayload
			if err := decode(step, &p); err != nil {
				return nil, err
			}
			args, err := resolve(ctx, store, p.ArgsRef)
			if err != nil {
				return nil, err
			}
			turns = append(turns, Turn{
				Kind:   TurnToolUse,
				CallID: CallID(step.RunID, step.Seq),
				Tool:   p.Tool,
				Args:   args,
			})

		case domain.StepToolReturned:
			var p domain.ToolReturnedPayload
			if err := decode(step, &p); err != nil {
				return nil, err
			}
			content, err := resolve(ctx, store, p.ResultRef)
			if err != nil {
				return nil, err
			}
			if p.Failed && len(content) == 0 {
				content = []byte("the tool failed: " + p.ErrorCode)
			}
			originalBytes := int64(len(content))
			contentDigest := digest(content)
			var elided int64
			if !p.Failed {
				content = compactToolResult(p.Tool, content, &elided)
			}
			turns = append(turns, Turn{
				Kind: TurnToolResult,
				// Pairs with the call one step earlier.
				CallID:        CallID(step.RunID, step.Seq-1),
				Tool:          p.Tool,
				Failed:        p.Failed,
				Content:       content,
				Elided:        elided,
				OriginalBytes: originalBytes,
				ContentDigest: contentDigest,
			})

		case domain.StepGateDecided:
			var p domain.GateDecidedPayload
			if err := decode(step, &p); err != nil {
				return nil, err
			}
			// A refusal is only useful to the model if it learns why; an
			// allowed call needs no narration.
			if p.Verdict.Executable() {
				continue
			}
			if p.Verdict == domain.VerdictDuplicate {
				turns = append(turns, Turn{
					Kind: TurnNote,
					Text: fmt.Sprintf("The platform skipped the call to %s (%s): %s. Continue without calling it again.",
						p.Tool, p.Rule, p.Reason),
				})
				continue
			}
			turns = append(turns, Turn{
				Kind: TurnNote,
				Text: fmt.Sprintf("The platform refused the call to %s (%s): %s. Choose a different approach.",
					p.Tool, p.Rule, p.Reason),
			})

		case domain.StepApprovalDecided:
			var p domain.ApprovalDecidedPayload
			if err := decode(step, &p); err != nil {
				return nil, err
			}
			if p.Approved {
				turns = append(turns, Turn{Kind: TurnNote, Text: "A human approved the pending action. Proceed."})
			} else {
				turns = append(turns, Turn{Kind: TurnNote,
					Text: "A human refused the pending action. Do not retry it; choose another approach or finish."})
			}
		}
	}
	boundToolResultTranscript(turns)
	return turns, nil
}

// CallID derives the identifier that pairs a tool call with its result.
// Deterministic in the run and sequence, so rebuilding the transcript from the
// ledger produces the same pairing every time.
func CallID(runID domain.RunID, seq int64) string {
	return fmt.Sprintf("call_%s_%d", runID, seq)
}

func resolve(ctx context.Context, store ContentStore, ref string) ([]byte, error) {
	if ref == "" || store == nil {
		return nil, nil
	}
	data, err := store.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("engine: resolve %s: %w", ref, err)
	}
	return data, nil
}
