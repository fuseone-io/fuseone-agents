// Package blocks reads an agent's instructions as blocks and writes them back
// without losing a byte (NT-018, contract 2).
//
// A line that is exactly a label, on its own, opens a block; anything before
// the first label is prose, kept whole. That is the reading the console's
// editor already makes. What this adds is the other direction: each block
// keeps the raw fragment it was read from, so an untouched block is written
// back as it was, and opening an agent and publishing it unchanged keeps its
// version. Only a block somebody changed is written in the canonical form,
// with its label in the language of whoever changed it.
//
// Pure: no I/O, so the draft, the validator and their tests share one codec.
package blocks

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Kind is what a block is for. Every kind may repeat.
type Kind string

const (
	Prose      Kind = "prose"
	Objective  Kind = "objective"
	HowToAct   Kind = "howToAct"
	WhenToStop Kind = "whenToStop"
	Never      Kind = "never"
	HowToReply Kind = "howToReply"
)

// labels are how each labelled kind is written, Portuguese first, in every
// language the console speaks. The label is part of what the model reads, and
// every language is recognised on the way back.
var labels = map[Kind][2]string{
	Objective:  {"Objetivo", "Purpose"},
	HowToAct:   {"Como agir", "How to act"},
	WhenToStop: {"Quando parar", "When to stop"},
	Never:      {"Nunca", "Never"},
	HowToReply: {"Como responder", "How to reply"},
}

// order is the kinds in the order the console reads them, for a stable lookup.
var order = []Kind{Objective, HowToAct, WhenToStop, Never, HowToReply}

// ErrUnknownBlock is an operation naming a block, a kind or a position the
// document does not have.
var ErrUnknownBlock = errors.New("blocks: no such block, kind or position")

// Block is one block of the instructions.
type Block struct {
	// ID is stable for the life of a draft: the parse gives b1, b2, … in
	// order, and a new block takes the next number.
	ID   string
	Kind Kind
	// Label is the heading the block is written under: as read, or the
	// locale's label for a block somebody changed. Empty for prose.
	Label string
	// Text is the block's content, trimmed.
	Text string
	// Raw is the exact fragment the block was read from, label line and
	// trailing separator included. Empty once the block is changed, which is
	// what makes it render canonically.
	Raw string
}

// KindOf answers whether a kind is one of the six.
func KindOf(kind Kind) bool { return kind == Prose || labels[kind] != [2]string{} }

// LabelOf is the label a kind is written under in a locale. Empty for prose.
func LabelOf(kind Kind, locale string) string {
	pair, ok := labels[kind]
	if !ok {
		return ""
	}
	if strings.HasPrefix(locale, "pt") {
		return pair[0]
	}
	return pair[1]
}

// Parse reads instructions, as spec.Parse leaves them, into blocks whose raw
// fragments concatenate back to the same bytes.
func Parse(instructions string) []Block {
	var out []Block
	var current *Block
	var body strings.Builder
	flush := func() {
		if current == nil {
			return
		}
		current.Text = strings.TrimSpace(body.String())
		out = append(out, *current)
		body.Reset()
	}
	for _, line := range strings.SplitAfter(instructions, "\n") {
		if line == "" {
			continue
		}
		heading := strings.TrimSpace(line)
		if kind, ok := labelled(heading); ok {
			flush()
			current = &Block{ID: idOf(len(out) + 1), Kind: kind, Label: heading, Raw: line}
			continue
		}
		if current == nil {
			current = &Block{ID: idOf(1), Kind: Prose}
		}
		current.Raw += line
		body.WriteString(line)
	}
	flush()
	return out
}

func labelled(line string) (Kind, bool) {
	for _, kind := range order {
		for _, label := range labels[kind] {
			if strings.EqualFold(line, label) {
				return kind, true
			}
		}
	}
	return "", false
}

func idOf(n int) string { return "b" + strconv.Itoa(n) }

/*
Render writes blocks back as instructions.

An untouched block is its raw fragment. Two untouched blocks that were
adjacent stay adjacent, whatever separated them. A changed or new block is
`label`, a newline and its text, set apart by a blank line from whatever comes
before it, which is never rewritten: separation is only ever appended. The
outer space is trimmed, as spec.Parse trims it, so what this returns is what
publication keeps.
*/
func Render(doc []Block) string {
	var out strings.Builder
	previousRaw := false
	for _, b := range doc {
		piece, raw := b.Raw, b.Raw != ""
		if !raw {
			piece = canonical(b)
		}
		if piece == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString(separator(out.String(), previousRaw && raw))
		}
		out.WriteString(piece)
		previousRaw = raw
	}
	return strings.TrimSpace(out.String())
}

func canonical(b Block) string {
	if b.Text == "" {
		// A label with nothing under it is a heading the model cannot act on.
		return ""
	}
	if b.Label == "" {
		return b.Text
	}
	return b.Label + "\n" + b.Text
}

// separator is what has to be appended before the next piece.
func separator(written string, bothRaw bool) string {
	switch {
	case bothRaw && strings.HasSuffix(written, "\n"):
		return ""
	case endsWithBlankLine(written):
		return ""
	case strings.HasSuffix(written, "\n"):
		return "\n"
	default:
		return "\n\n"
	}
}

func endsWithBlankLine(s string) bool {
	s = strings.TrimRight(s, " \t\r")
	if !strings.HasSuffix(s, "\n") {
		return false
	}
	s = strings.TrimRight(s[:len(s)-1], " \t\r")
	return strings.HasSuffix(s, "\n")
}

// Insert adds a block of a kind at a position, written in the locale given.
func Insert(doc []Block, at int, kind Kind, text, locale string) ([]Block, error) {
	if at < 0 || at > len(doc) || !KindOf(kind) {
		return nil, fmt.Errorf("%w: insert %s at %d", ErrUnknownBlock, kind, at)
	}
	b := Block{ID: nextID(doc), Kind: kind, Label: LabelOf(kind, locale), Text: strings.TrimSpace(text)}
	return slices.Insert(slices.Clone(doc), at, b), nil
}

// Replace sets a block's text. Unchanged text leaves the block untouched, so
// its bytes stay; changed text writes it canonically, under the locale's label.
func Replace(doc []Block, id, text, locale string) ([]Block, error) {
	i := indexOf(doc, id)
	if i < 0 {
		return nil, fmt.Errorf("%w: replace %s", ErrUnknownBlock, id)
	}
	out := slices.Clone(doc)
	text = strings.TrimSpace(text)
	if text == out[i].Text {
		return out, nil
	}
	out[i] = Block{ID: id, Kind: out[i].Kind, Label: LabelOf(out[i].Kind, locale), Text: text}
	return out, nil
}

// Remove takes a block out.
func Remove(doc []Block, id string) ([]Block, error) {
	i := indexOf(doc, id)
	if i < 0 {
		return nil, fmt.Errorf("%w: remove %s", ErrUnknownBlock, id)
	}
	return slices.Delete(slices.Clone(doc), i, i+1), nil
}

// Move puts a block at a position, counted in the document as it ends up.
func Move(doc []Block, id string, to int) ([]Block, error) {
	i := indexOf(doc, id)
	if i < 0 || to < 0 || to >= len(doc) {
		return nil, fmt.Errorf("%w: move %s to %d", ErrUnknownBlock, id, to)
	}
	b := doc[i]
	out := slices.Delete(slices.Clone(doc), i, i+1)
	return slices.Insert(out, to, b), nil
}

func indexOf(doc []Block, id string) int {
	return slices.IndexFunc(doc, func(b Block) bool { return b.ID == id })
}

// nextID is one past the highest numbered id in the document. A draft that
// must never reuse the id of a block it removed keeps its own counter.
func nextID(doc []Block) string {
	highest := 0
	for _, b := range doc {
		if n, err := strconv.Atoi(strings.TrimPrefix(b.ID, "b")); err == nil && n > highest {
			highest = n
		}
	}
	return idOf(highest + 1)
}
