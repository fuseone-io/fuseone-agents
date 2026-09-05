package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	channelInputCompactAfter = 32 << 10
	inputFieldCompactAfter   = 16 << 10
	inputFieldHeadBytes      = 10 << 10
	inputFieldTailBytes      = 4 << 10
)

func compactRunInputForTranscript(trigger string, content []byte) ([]byte, string) {
	if trigger != "channel" || len(content) <= channelInputCompactAfter {
		return content, ""
	}
	var input any // JSON input can contain any value allowed by the channel contract.
	if err := json.Unmarshal(content, &input); err != nil {
		return compactRawChannelInput(content), channelInputCompactionNote(content, nil)
	}
	changes := []map[string]any{}
	compacted := compactJSONStrings(input, "", &changes)
	if len(changes) == 0 {
		return content, ""
	}
	obj, ok := compacted.(map[string]any)
	if !ok {
		return compactRawChannelInput(content), channelInputCompactionNote(content, nil)
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return compactRawChannelInput(content), channelInputCompactionNote(content, nil)
	}
	return out, channelInputCompactionNote(content, changes)
}

func runInputForTranscript(trigger string, content []byte) ([]byte, string) {
	if trigger == "channel" {
		if projected, ok := channelAskForTranscript(content); ok {
			if len(projected) > channelInputCompactAfter {
				return compactRawChannelInput(projected), channelInputCompactionNote(content, nil)
			}
			return projected, ""
		}
	}
	return compactRunInputForTranscript(trigger, content)
}

type channelAskTranscript struct {
	Subject *struct {
		Kind string `json:"kind"`
		Run  string `json:"run"`
	} `json:"subject,omitempty"`
	Text   string                   `json:"text"`
	Thread *channelThreadTranscript `json:"thread,omitempty"`
}

type channelThreadTranscript struct {
	Messages []struct {
		Source string `json:"source,omitempty"`
		Text   string `json:"text"`
	} `json:"messages,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
}

/*
channelAskForTranscript is what the model sees of an ask made in a channel.

The envelope does not travel. asked_by, the vendor account, the conversation
and the thread references are how the platform routes and audits an ask, and
none of them is the question — a model that reads them starts treating a Slack
user id as an instruction.

Every ask that parses is projected, however little it turns out to say. The
first version of this chose by the text, so a mention whose question is the
thread it arrived on — empty text, real context — fell out of the projection
and the whole envelope went to the model: the exact leak this exists to
prevent, reached by the one shape nobody had thought about. Choosing by any
other field would have the same defect waiting behind it, so nothing chooses:
an ask with nothing in it projects to nothing, which is honest and carries no
metadata. Only content that is not an ask at all — not JSON — falls back.
*/
func channelAskForTranscript(content []byte) ([]byte, bool) {
	var ask channelAskTranscript
	if err := json.Unmarshal(content, &ask); err != nil {
		return nil, false
	}

	parts := make([]string, 0, 4)
	if ask.Text != "" {
		parts = append(parts, ask.Text)
	}
	if ask.Subject != nil && ask.Subject.Kind == "run" && ask.Subject.Run != "" {
		parts = append(parts, fmt.Sprintf("Thread subject: run %s.", ask.Subject.Run))
	}
	parts = append(parts, threadContextParts(ask.Thread)...)
	return []byte(strings.Join(parts, "\n\n")), true
}

// threadContextParts is the surrounding evidence, as sections.
//
// Sections rather than one appended string: with no text of its own, an ask
// built by appending would open with the blank lines meant to separate it from
// words that were never there.
func threadContextParts(thread *channelThreadTranscript) []string {
	if thread == nil {
		return nil
	}
	var parts []string
	if thread.Unavailable != "" {
		parts = append(parts, "Thread context unavailable: "+thread.Unavailable)
	}
	if len(thread.Messages) > 0 {
		var b strings.Builder
		b.WriteString("Earlier thread messages:")
		for _, msg := range thread.Messages {
			b.WriteString("\n- ")
			if msg.Source != "" {
				b.WriteString(msg.Source)
				b.WriteString(": ")
			}
			b.WriteString(msg.Text)
		}
		parts = append(parts, b.String())
	}
	if thread.Truncated {
		parts = append(parts, "Earlier thread messages were truncated.")
	}
	return parts
}

func channelInputCompactionNote(content []byte, fields []map[string]any) string {
	var b strings.Builder
	b.WriteString("FuseOne compacted the channel input before sending it to the model.\n")
	fmt.Fprintf(&b, "Stored input: %d bytes, digest %s.\n", len(content), digest(content))
	b.WriteString("Only the beginning and end of compacted content are shown. Do not treat omitted middle as absent; use a narrower ask or fetch the original source if this is not enough.")
	if len(fields) > 0 {
		b.WriteString("\nCompacted fields:")
		for _, field := range fields {
			fmt.Fprintf(&b, "\n- %v: %v bytes stored, %v bytes shown",
				field["path"], field["original_bytes"], field["shown_bytes"])
		}
	}
	return b.String()
}

func compactJSONStrings(v any, path string, changes *[]map[string]any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			x[k] = compactJSONStrings(child, jsonPath(path, k), changes)
		}
		return x
	case []any:
		for i, child := range x {
			x[i] = compactJSONStrings(child, fmt.Sprintf("%s[%d]", path, i), changes)
		}
		return x
	case string:
		if len(x) <= inputFieldCompactAfter {
			return x
		}
		compacted := compactTextField(x)
		*changes = append(*changes, map[string]any{
			"path": path, "original_bytes": len(x), "shown_bytes": len(compacted),
		})
		return compacted
	default:
		return v
	}
}

func compactTextField(text string) string {
	head := utf8Prefix([]byte(text), inputFieldHeadBytes)
	tail := utf8Suffix([]byte(text), inputFieldTailBytes)
	var b strings.Builder
	fmt.Fprintf(&b, "--- first %d bytes ---\n%s\n\n", len(head), head)
	fmt.Fprintf(&b, "--- omitted %d bytes ---\n\n", max(0, len(text)-len(head)-len(tail)))
	fmt.Fprintf(&b, "--- last %d bytes ---\n%s", len(tail), tail)
	return b.String()
}

func compactRawChannelInput(content []byte) []byte {
	head := utf8Prefix(content, inputFieldHeadBytes)
	tail := utf8Suffix(content, inputFieldTailBytes)
	var b strings.Builder
	fmt.Fprintf(&b, "--- first %d bytes ---\n%s\n\n", len(head), head)
	fmt.Fprintf(&b, "--- omitted %d bytes ---\n\n", max(0, len(content)-len(head)-len(tail)))
	fmt.Fprintf(&b, "--- last %d bytes ---\n%s", len(tail), tail)
	return []byte(b.String())
}

func jsonPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}
