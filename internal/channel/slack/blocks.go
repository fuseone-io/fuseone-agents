package slack

import (
	"encoding/json"
	"sort"
	"strings"
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

**It is deliberately not a Slack renderer.** It collects the string fields that
carry human words wherever they appear, which is what an agent needs to read;
it does not reproduce layout, and it does not try to keep up with block kinds
Slack has not shipped yet. The whole payload is still recorded on the arrival,
so an auditor reading the thing itself loses nothing.
*/

const (
	// Deep enough for the shapes alerting systems actually post — attachment,
	// block, field, rich-text element — and shallow enough that a payload
	// nesting forever cannot walk this forever with it.
	maxBlockDepth = 12
	// What reaches the run input. A Slack message can be far larger than
	// anything worth asking a model about, and the bound is here rather than
	// downstream because this is where the cost is decided.
	maxBlockTextBytes = 4 << 10
)

// blockTextKeys are the fields Slack puts human words in. `value` and `title`
// belong to attachment fields, `fallback` to the attachment itself.
var blockTextKeys = map[string]bool{
	"text": true, "fallback": true, "title": true, "value": true,
}

// messageText is what the platform reads as the message.
func messageText(e envelope) string {
	if strings.TrimSpace(e.Event.Text) != "" {
		return e.Event.Text
	}
	return blockText(e.Event.Blocks, e.Event.Attachments)
}

func blockText(sources ...json.RawMessage) string {
	found := make([]string, 0, 8)
	seen := map[string]bool{}
	size := 0
	for _, raw := range sources {
		if len(raw) == 0 {
			continue
		}
		var decoded any // Slack's block payload is any JSON it chooses to send.
		if err := json.Unmarshal(raw, &decoded); err != nil {
			continue
		}
		collectBlockText(decoded, 0, &found, seen, &size)
	}
	return strings.Join(found, "\n")
}

/*
collectBlockText walks a decoded payload gathering the words in it.

Keys are visited in sorted order, never map order: two readings of one payload
have to produce one string, or the same alert reaches the model differently on
different days and nobody can reproduce what an agent was asked.

Repeats are dropped. An attachment states its `fallback` and its `text`, which
are usually the same sentence, and an agent reading it twice is being told it
matters twice.
*/
func collectBlockText(node any, depth int, found *[]string, seen map[string]bool, size *int) {
	if depth > maxBlockDepth || *size >= maxBlockTextBytes {
		return
	}
	switch value := node.(type) {
	case []any:
		for _, item := range value {
			collectBlockText(item, depth+1, found, seen, size)
		}
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if words, ok := value[key].(string); ok {
				if blockTextKeys[key] {
					keepWords(words, found, seen, size)
				}
				continue
			}
			collectBlockText(value[key], depth+1, found, seen, size)
		}
	}
}

func keepWords(words string, found *[]string, seen map[string]bool, size *int) {
	words = strings.TrimSpace(words)
	if words == "" || seen[words] {
		return
	}
	if room := maxBlockTextBytes - *size; len(words) > room {
		words = words[:room]
	}
	seen[words] = true
	*found = append(*found, words)
	*size += len(words)
}
