package slack

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

/*
Reading the words out of a message that has none in its text field.

Slack messages carry their content twice over: `text` for clients that cannot
render anything, and `blocks` or `attachments` for those that can. Alerting
systems posting through an integration routinely fill only the second, so a
platform that read `text` alone turned a real alert into a run with an empty
input — a model call paid for with no question in it.

This reads the second, and only when the first is empty. A message that has
both said what it wanted to say in `text`, and appending the rendered version
would hand the model the same alert twice.

**It walks the shapes rather than the keys.** The first version collected every
string under a handful of key names wherever they appeared, which cannot tell a
label from a control's callback data: a button's `value` is read back by the
app when somebody clicks it, is never on screen, and was going to the model. So
each shape is read for the fields that hold words a person can see, and a
`value` counts only where it means one — one half of an attachment field's
label-and-value pair.

**It is not a Slack renderer.** It does not reproduce layout and does not try
to keep up with block kinds Slack has not shipped yet. The whole payload is
recorded on the arrival, so an auditor reading the thing itself loses nothing,
and an integration whose shape is not read here makes its conversation go quiet
rather than start runs on half an alert.
*/

const (
	// Deep enough for the shapes alerting systems actually post — attachment,
	// block, field, rich-text section, element — and shallow enough that a
	// payload nesting forever cannot walk this forever with it.
	maxBlockDepth = 12
	// What reaches the run input, separators and the note below included. A
	// Slack message can be far larger than anything worth asking a model
	// about, and the bound is here rather than downstream because this is
	// where the cost is decided.
	maxBlockTextBytes = 4 << 10
	// Said to the model rather than left to be inferred. A reader told nothing
	// about the omission reasons from a sentence that stops mid-fact.
	truncationNote = "\n… the rest of this message was not read"
)

// messageText is what the platform reads as the message.
func messageText(e envelope) string {
	if strings.TrimSpace(e.Event.Text) != "" {
		return e.Event.Text
	}
	var w words
	w.blocks(decodeList(e.Event.Blocks), 0)
	w.attachments(decodeList(e.Event.Attachments), 0)
	return w.String()
}

// words is what has been read so far, and whether anything was left behind.
type words struct {
	found     []string
	size      int
	truncated bool
}

// String is the message as read, and nothing when none of it was.
//
// The note is not a message. Returned on its own it is non-empty text, which
// passes for a question downstream and opens a paid run whose entire content
// is the platform apologising. It only means something beside words that were
// actually read.
func (w *words) String() string {
	if len(w.found) == 0 {
		return ""
	}
	out := strings.Join(w.found, "\n")
	if w.truncated {
		out += truncationNote
	}
	return out
}

/*
add keeps one piece of visible text, inside the ceiling.

The ceiling covers the separators and the note as well as the words: a message
assembled from five hundred small blocks is as expensive as one long one, and
counting only the words let it past. Room for the note is reserved whether or
not it ends up being used, so the bound holds either way.

What arrives here is already normalised: visibleText is the one place that
decides what a Slack text field says, so trimming again here would be a second
definition of it and the two would drift.

The cut lands on a rune boundary. A prefix taken by byte count splits a
multi-byte character and produces a string PostgreSQL will not store — so a
valid Slack delivery fails to be written down, and the sender retries it
forever.
*/
func (w *words) add(said string) {
	if said == "" || w.truncated {
		return
	}
	separator := 0
	if len(w.found) > 0 {
		separator = 1
	}
	room := maxBlockTextBytes - len(truncationNote) - w.size - separator
	if room <= 0 {
		w.truncated = true
		return
	}
	if len(said) > room {
		said = said[:runeBoundary(said, room)]
		w.truncated = true
		if said == "" {
			return
		}
	}
	w.found = append(w.found, said)
	w.size += separator + len(said)
}

// runeBoundary is the largest cut at or below max that keeps the string valid.
func runeBoundary(said string, max int) int {
	cut := max
	for cut > 0 && !utf8.RuneStart(said[cut]) {
		cut--
	}
	return cut
}

/*
blocks reads Slack's Block Kit layout.

A block states its own text, its fields, and whatever elements it arranges.
Interactive elements contribute their label and nothing else — `value`,
`action_id` and `url` are the app's own wiring, and none of them is on screen.
*/
func (w *words) blocks(items []any, depth int) {
	if len(items) == 0 || w.tooDeep(depth) {
		return
	}
	for _, item := range items {
		block := asObject(item)
		if block == nil {
			continue
		}
		w.add(visibleText(block["text"]))
		w.add(visibleText(block["title"]))
		for _, field := range asList(block["fields"]) {
			w.add(visibleText(field))
		}
		w.elements(asList(block["elements"]), depth+1)
		if accessory := asObject(block["accessory"]); accessory != nil {
			w.add(visibleText(accessory["text"]))
		}
	}
}

// elements reads what a block arranges: rich-text runs, context lines, and
// controls. Only ever the visible label, and never the wiring behind it.
func (w *words) elements(items []any, depth int) {
	if len(items) == 0 || w.tooDeep(depth) {
		return
	}
	for _, item := range items {
		element := asObject(item)
		if element == nil {
			continue
		}
		w.add(visibleText(element["text"]))
		w.elements(asList(element["elements"]), depth+1)
	}
}

/*
attachments reads Slack's older attachment shape.

`fallback` is a plain-text rendering of the attachment's own text, so the two
are said once. That is the only repeat dropped: the same word in two different
blocks is two facts — two hosts reported down is not one host reported down —
and collapsing them deletes a state and leaves the reader pairing labels with
the wrong values.
*/
func (w *words) attachments(items []any, depth int) {
	if len(items) == 0 || w.tooDeep(depth) {
		return
	}
	for _, item := range items {
		attachment := asObject(item)
		if attachment == nil {
			continue
		}
		fallback := visibleText(attachment["fallback"])
		w.add(fallback)
		for _, key := range []string{"pretext", "title", "text"} {
			if said := visibleText(attachment[key]); said != fallback {
				w.add(said)
			}
		}
		for _, item := range asList(attachment["fields"]) {
			field := asObject(item)
			w.add(visibleText(field["title"]))
			// The one place a `value` is somebody's words rather than an
			// app's callback data.
			w.add(visibleText(field["value"]))
		}
		w.blocks(asList(attachment["blocks"]), depth+1)
	}
}

// tooDeep stops the walk, and records that it stopped: content beyond the
// bound was omitted like any other, and the reader is told.
//
// Its three callers ask only once there is something to descend into. A leaf
// sitting exactly on the bound has no children to leave behind, and announcing
// an omission that never happened sends a reader looking for content that was
// always complete. Only nested elements can reach the bound today — blocks and
// attachments are entered once from the message and once from an attachment —
// so the guard is uniform rather than three separately reasoned cases.
func (w *words) tooDeep(depth int) bool {
	if depth <= maxBlockDepth {
		return false
	}
	w.truncated = true
	return true
}

// visibleText reads a Slack text object, or a plain string where Slack uses
// one. Anything else is a shape this does not claim to understand.
//
// Normalised here rather than only where it is kept, because callers compare
// what comes back: an attachment whose fallback is its own text with padding
// around it is the same sentence, and comparing the raw forms said it twice.
func visibleText(node any) string {
	switch value := node.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		said, _ := value["text"].(string)
		return strings.TrimSpace(said)
	default:
		return ""
	}
}

func decodeList(raw json.RawMessage) []any {
	if len(raw) == 0 {
		return nil
	}
	var decoded []any // Slack sends whatever JSON it chooses inside these.
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func asList(node any) []any {
	list, _ := node.([]any)
	return list
}

func asObject(node any) map[string]any {
	object, _ := node.(map[string]any)
	return object
}
