package engine

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

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
				turns = append(turns, Turn{Kind: TurnInput, Text: string(text)})
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
			if !p.Failed {
				content = compactToolResultForTranscript(p.Tool, content)
			}
			turns = append(turns, Turn{
				Kind: TurnToolResult,
				// Pairs with the call one step earlier.
				CallID:  CallID(step.RunID, step.Seq-1),
				Tool:    p.Tool,
				Failed:  p.Failed,
				Content: content,
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
	return turns, nil
}

const (
	toolResultCompactAfter = 32 << 10
	toolResultHeadBytes    = 16 << 10
	toolResultTailBytes    = 8 << 10
)

func compactToolResultForTranscript(tool domain.ToolID, content []byte) []byte {
	if len(content) <= toolResultCompactAfter || !compactableObservabilityTool(tool) {
		return content
	}
	head := utf8Prefix(content, toolResultHeadBytes)
	tail := utf8Suffix(content, toolResultTailBytes)

	var b strings.Builder
	fmt.Fprintf(&b, "FuseOne compacted this %s result before sending it back to the model.\n", tool)
	fmt.Fprintf(&b, "Stored result: %d bytes, digest %s.\n", len(content), digest(content))
	b.WriteString("Only the beginning and end are shown here. Do not treat the omitted middle as absent; call a narrower query if this is not enough.\n\n")
	fmt.Fprintf(&b, "--- first %d bytes ---\n%s\n\n", len(head), head)
	fmt.Fprintf(&b, "--- omitted %d bytes ---\n\n", max(0, len(content)-len(head)-len(tail)))
	fmt.Fprintf(&b, "--- last %d bytes ---\n%s", len(tail), tail)
	return []byte(b.String())
}

func compactableObservabilityTool(tool domain.ToolID) bool {
	name := string(tool)
	if !strings.HasPrefix(name, "grafana.") {
		return false
	}
	return strings.HasPrefix(name, "grafana.query_loki") ||
		strings.HasPrefix(name, "grafana.query_prometheus")
}

func utf8Prefix(content []byte, limit int) string {
	if len(content) <= limit {
		return string(content)
	}
	part := content[:limit]
	for len(part) > 0 && !utf8.Valid(part) {
		part = part[:len(part)-1]
	}
	return string(part)
}

func utf8Suffix(content []byte, limit int) string {
	if len(content) <= limit {
		return string(content)
	}
	part := content[len(content)-limit:]
	for len(part) > 0 && !utf8.Valid(part) {
		part = part[1:]
	}
	return string(part)
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
