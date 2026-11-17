package conversation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
)

const perMillion = 1_000_000

/*
Reserve is a turn's worst case, in micros, priced at the model's configured
rates: every byte of the prompt counted as a token (a token is never less than
a byte), and the maximum output. An unknown price, or an input that alone
exceeds MaxTurnMicros, is refused before anything is reserved or called.
*/
func Reserve(prompt string, price model.Prices, known bool) (int64, error) {
	if !known {
		return 0, ErrNoPrice
	}
	input := int64(len(prompt)) * price.InputMicros / perMillion
	if input > MaxTurnMicros {
		return 0, fmt.Errorf("%w: its input alone would cost %d micros, at most %d", ErrTurnTooLarge, input, MaxTurnMicros)
	}
	return input + MaxOutputTokens*price.OutputMicros/perMillion, nil
}

// Offer is what the draft's scope offers: the tools usable there and the
// providers configured there.
type Offer struct {
	Tools     []domain.ToolEntry
	Providers []string
}

type projectedBlock struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Text  string `json:"text"`
}

type projectedTool struct {
	ID          string `json:"id"`
	Effect      string `json:"effect"`
	Description string `json:"description,omitempty"`
}

/*
Project is what the model is shown of a draft: the whole definition in the
publication's own format, the blocks with their ids, what publication still
needs, and what the scope offers. No secret and no address: a definition holds
none, and a tool is shown by id, effect and description, never its server.
Larger than MaxProjectionBytes, the turn is refused rather than cut.
*/
func Project(d draft.Draft, missing []string, offer Offer) (string, error) {
	s := d.Spec
	s.Instructions = ""
	definition, err := spec.Render(s)
	if err != nil {
		return "", fmt.Errorf("conversation: render the draft: %w", err)
	}
	out := struct {
		Definition string           `json:"definition"`
		Blocks     []projectedBlock `json:"blocks"`
		Missing    []string         `json:"missing"`
		Tools      []projectedTool  `json:"toolsOffered"`
		Providers  []string         `json:"providersOffered"`
	}{Definition: string(definition), Missing: missing, Providers: slices.Clone(offer.Providers)}
	for _, b := range d.Blocks {
		out.Blocks = append(out.Blocks, projectedBlock{ID: b.ID, Kind: string(b.Kind), Label: b.Label, Text: b.Text})
	}
	for _, t := range offer.Tools {
		out.Tools = append(out.Tools, projectedTool{ID: string(t.ID), Effect: t.Effect.String(), Description: t.Description})
	}
	encoded, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		return "", fmt.Errorf("conversation: encode the projection: %w", err)
	}
	if len(encoded) > MaxProjectionBytes {
		return "", fmt.Errorf("%w: the draft's projection is %d bytes, at most %d", ErrTurnTooLarge, len(encoded), MaxProjectionBytes)
	}
	return string(encoded), nil
}

// Window is the conversation sent with a turn: the newest turns that fit
// WindowTurns and WindowBytes, oldest first.
func Window(turns []drafts.Turn) []drafts.Turn {
	var kept []drafts.Turn
	used := 0
	for i := len(turns) - 1; i >= 0 && len(kept) < WindowTurns; i-- {
		size := len(turns[i].Message) + len(turns[i].Reply)
		if used+size > WindowBytes {
			continue
		}
		used += size
		kept = append(kept, turns[i])
	}
	slices.Reverse(kept)
	return kept
}

// Reply is the model's answer: what it says, the operations it proposes, and
// how many it proposed that could not be read or went past the cap.
type Reply struct {
	Text      string
	Proposals []draft.Operation
	Dropped   int
}

// ReadReply reads the answer's JSON object: {"reply": text, "proposals":
// [operation bodies]}. Proposals past MaxProposalsPerTurn, and any that is not
// an operation, are dropped and counted.
func ReadReply(text string) (Reply, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return Reply{}, ErrUnreadableReply
	}
	var raw struct {
		Reply     string            `json:"reply"`
		Proposals []json.RawMessage `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return Reply{}, fmt.Errorf("%w: %v", ErrUnreadableReply, err)
	}
	out := Reply{Text: raw.Reply}
	for _, body := range raw.Proposals {
		op, err := draft.ParseBody(body)
		if err != nil || len(out.Proposals) == MaxProposalsPerTurn {
			out.Dropped++
			continue
		}
		out.Proposals = append(out.Proposals, op)
	}
	return out, nil
}
