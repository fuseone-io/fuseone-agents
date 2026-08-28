package memory

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/fuseone/agents/internal/domain"
)

// What the model sent, and what the schema says it may send.
//
// Decoding refuses rather than repairs. An argument the platform guessed at is
// an argument the model never chose, and a memory written from one would be a
// fact nobody said.

type findArgs struct {
	Kind      string `json:"kind,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Signature string `json:"signature,omitempty"`
	Search    string `json:"search,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type suggestArgs struct {
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	Signature string `json:"signature"`
	Claim     string `json:"claim"`
}

func decodeFindArgs(raw []byte) (findArgs, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return findArgs{}, true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var args findArgs
	if err := dec.Decode(&args); err != nil {
		return findArgs{}, false
	}
	return args, dec.Decode(&struct{}{}) == io.EOF
}

func decodeSuggestArgs(raw []byte) (suggestArgs, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var args suggestArgs
	if err := dec.Decode(&args); err != nil {
		return suggestArgs{}, false
	}
	return args, dec.Decode(&struct{}{}) == io.EOF
}

func memoryFindSchema() map[string]any {
	text := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return map[string]any{
		"kind":      text("Optional assertion kind to match exactly."),
		"subject":   text("Optional subject to match exactly."),
		"signature": text("Optional signature to match exactly."),
		"search":    text("Optional space-separated terms to search across subject, signature and claim. Only a small normalized term budget is used; the result reports when extra terms were omitted."),
		"limit": map[string]any{
			"type": "integer", "minimum": 1, "maximum": domain.MaxMemoryFindLimit,
			"description": "Maximum assertions to return.",
		},
	}
}

func memorySuggestSchema() map[string]any {
	text := func(description string, max int) map[string]any {
		return map[string]any{"type": "string", "maxLength": max, "description": description}
	}
	return map[string]any{
		"kind":      text("Stable assertion kind.", domain.MaxMemoryKindBytes),
		"subject":   text("Thing this assertion is about.", domain.MaxMemorySubjectBytes),
		"signature": text("Stable key for the repeated situation.", domain.MaxMemorySignatureBytes),
		"claim":     text("Small falsifiable claim to remember.", domain.MaxMemoryClaimBytes),
	}
}
