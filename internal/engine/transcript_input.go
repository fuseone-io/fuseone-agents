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
	Text   string `json:"text"`
	Thread *struct {
		Messages []struct {
			Source string `json:"source,omitempty"`
			Text   string `json:"text"`
		} `json:"messages,omitempty"`
		Truncated   bool   `json:"truncated,omitempty"`
		Unavailable string `json:"unavailable,omitempty"`
	} `json:"thread,omitempty"`
}

func channelAskForTranscript(content []byte) ([]byte, bool) {
	var ask channelAskTranscript
	if err := json.Unmarshal(content, &ask); err != nil || ask.Text == "" {
		return nil, false
	}
	var b strings.Builder
	b.WriteString(ask.Text)
	if ask.Subject != nil && ask.Subject.Kind == "run" && ask.Subject.Run != "" {
		fmt.Fprintf(&b, "\n\nThread subject: run %s.", ask.Subject.Run)
	}
	appendThreadContext(&b, ask.Thread)
	return []byte(b.String()), true
}

func appendThreadContext(b *strings.Builder, thread *struct {
	Messages []struct {
		Source string `json:"source,omitempty"`
		Text   string `json:"text"`
	} `json:"messages,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
}) {
	if thread == nil {
		return
	}
	if thread.Unavailable != "" {
		fmt.Fprintf(b, "\n\nThread context unavailable: %s", thread.Unavailable)
	}
	if len(thread.Messages) > 0 {
		b.WriteString("\n\nEarlier thread messages:")
		for _, msg := range thread.Messages {
			b.WriteString("\n- ")
			if msg.Source != "" {
				b.WriteString(msg.Source)
				b.WriteString(": ")
			}
			b.WriteString(msg.Text)
		}
	}
	if thread.Truncated {
		b.WriteString("\n\nEarlier thread messages were truncated.")
	}
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
