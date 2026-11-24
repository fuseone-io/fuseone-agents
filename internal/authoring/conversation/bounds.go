package conversation

import (
	"bytes"
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
	Providers []ProviderOffer
}

// ProviderOffer is a provider configured in the draft's scope and the models it serves.
type ProviderOffer struct {
	Name   string
	Models []string
}

type projectedBlock struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Text  string `json:"text"`
}

type projectedProvider struct {
	Name   string   `json:"name"`
	Models []string `json:"models"`
	// MoreModels is how many models past MaxModelsPerProvider were left out.
	MoreModels int `json:"moreModels,omitempty"`
}

// projectProviders is each offered provider with the models it serves, at most
// MaxModelsPerProvider of them, and how many more there are.
func projectProviders(offered []ProviderOffer) []projectedProvider {
	out := make([]projectedProvider, 0, len(offered))
	for _, p := range offered {
		shown := p.Models[:min(len(p.Models), MaxModelsPerProvider)]
		out = append(out, projectedProvider{
			Name: p.Name, Models: append([]string{}, shown...), MoreModels: len(p.Models) - len(shown),
		})
	}
	return out
}

type projectedTool struct {
	ID          string `json:"id"`
	Effect      string `json:"effect"`
	Description string `json:"description,omitempty"`
}

/*
Project is what the model is shown of a draft: the whole definition in the
publication's own format, the blocks with their ids, what publication still
needs, the tools no step reaches, and what the scope offers. No secret and no address: a definition holds
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
		Definition string              `json:"definition"`
		Blocks     []projectedBlock    `json:"blocks"`
		Missing    []string            `json:"missing"`
		Unplaced   []domain.ToolID     `json:"unplaced,omitempty"`
		Tools      []projectedTool     `json:"toolsOffered"`
		Providers  []projectedProvider `json:"providersOffered"`
	}{
		Definition: string(definition), Missing: missing, Unplaced: d.Spec.Unplaced(),
		Providers: projectProviders(offer.Providers),
	}
	for _, b := range d.Blocks {
		out.Blocks = append(out.Blocks, projectedBlock{ID: b.ID, Kind: string(b.Kind), Label: b.Label, Text: b.Text})
	}
	for _, t := range offer.Tools {
		out.Tools = append(out.Tools, projectedTool{ID: string(t.ID), Effect: t.Effect.String(), Description: t.Description})
	}
	// Compact, and the author's text as written: indentation and HTML
	// escaping ("<" as six bytes) only spend the turn's budget, and a real
	// agent's instructions are full of both.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return "", fmt.Errorf("conversation: encode the projection: %w", err)
	}
	encoded := bytes.TrimRight(buf.Bytes(), "\n")
	if len(encoded) > MaxProjectionBytes {
		return "", fmt.Errorf("%w: the draft's projection is %d bytes, at most %d", ErrTurnTooLarge, len(encoded), MaxProjectionBytes)
	}
	return string(encoded), nil
}

// Window is the conversation sent with a turn: the newest turns that fit
// WindowTurns and WindowBytes, oldest first.
func Window(turns []drafts.Turn) []drafts.Turn {
	sizes := make([]int, len(turns))
	for i, t := range turns {
		sizes[i] = len(t.Message) + len(t.Reply)
	}
	kept := make([]drafts.Turn, 0, WindowTurns)
	for _, i := range WindowOf(sizes) {
		kept = append(kept, turns[i])
	}
	return kept
}

/*
WindowOf is the window's rule on sizes alone: given each turn's size in bytes, oldest first,
the indexes of the newest that fit WindowTurns and WindowBytes, oldest first. A turn too
large for what is left is skipped and an older one that fits is still taken, so the window
can be chosen from sizes before any turn's content is read (#422).
*/
func WindowOf(sizes []int) []int {
	var kept []int
	used := 0
	for i := len(sizes) - 1; i >= 0 && len(kept) < WindowTurns; i-- {
		if used+sizes[i] > WindowBytes {
			continue
		}
		used += sizes[i]
		kept = append(kept, i)
	}
	slices.Reverse(kept)
	return kept
}

// Reply is the model's answer: what it says, the operations it proposes, and
// how many it proposed that could not be read or went past the cap.
type Reply struct {
	Text      string
	Proposals []drafts.Proposed
	Dropped   int
}

// ReadReply reads the answer's JSON object: {"reply": text, "proposals":
// [operation bodies, each with an optional "reason"]}. Proposals past MaxProposalsPerTurn, and any that is not
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
		proposed, ok := readProposal(body)
		if !ok || len(out.Proposals) == MaxProposalsPerTurn {
			out.Dropped++
			continue
		}
		out.Proposals = append(out.Proposals, proposed)
	}
	return out, nil
}

// readProposal is one proposal of an answer: an operation as ParseBody reads
// it, and the model's reason beside it. A reason over MaxReasonBytes drops
// the proposal rather than being cut: a cut reason can say the opposite.
func readProposal(body json.RawMessage) (drafts.Proposed, bool) {
	op, err := draft.ParseBody(body)
	if err != nil {
		return drafts.Proposed{}, false
	}
	var said struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal(body, &said) != nil || len(said.Reason) > MaxReasonBytes {
		return drafts.Proposed{}, false
	}
	return drafts.Proposed{Op: op, Reason: strings.TrimSpace(said.Reason)}, true
}
