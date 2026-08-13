package slack

import (
	"fmt"

	"github.com/fuseone/agents/internal/channel"
)

/*
What the message looks like.

English, and deliberately so for now: a channel has many readers and no
session, so there is no locale to render in — the console knows who is looking
and this does not. When channels reach the console's settings this becomes the
conversation's own choice rather than the process's.

The run identifier is mono-spaced and never abbreviated. It is what somebody
pastes into the console, and a truncated one costs a round trip.
*/

func summary(m channel.Message) string {
	switch m.Event {
	case channel.EventParked:
		if m.Tool != "" {
			return fmt.Sprintf("%s is waiting for permission to run %s", m.Agent, m.Tool)
		}
		return fmt.Sprintf("%s is waiting for a decision", m.Agent)
	case channel.EventFailed:
		return fmt.Sprintf("%s stopped: %s", m.Agent, reasonOr(m.Reason, "no reason recorded"))
	case "test":
		// Says what it is. A test message that looked like a real one would
		// have somebody opening a run that does not exist.
		return "FuseOne Agents is connected to this conversation. This is a test."
	default:
		return fmt.Sprintf("%s finished", m.Agent)
	}
}

func blocks(m channel.Message) []any {
	out := []any{section(summary(m))}
	if m.RunID == "" {
		// A test carries no run, and a fact block naming an empty one would be
		// the first thing a reader tried to look up.
		if m.Scope.Area != "" {
			out = append(out, context_(fmt.Sprintf("Runs in *%s* report here.", m.Scope.Area)))
		}
		return out
	}

	facts := []string{"*Run*\n`" + string(m.RunID) + "`"}
	if m.Scope.Area != "" {
		facts = append(facts, fmt.Sprintf("*Area*\n%s", m.Scope.Area))
	}
	if m.Tool != "" {
		facts = append(facts, "*Action*\n`"+m.Tool+"`")
	}
	if m.Reason != "" {
		facts = append(facts, fmt.Sprintf("*Reason*\n%s", m.Reason))
	}
	out = append(out, fields(facts))

	if m.Link != "" {
		// A link rather than a button. A button implies this message can act,
		// and in this stage nothing a conversation says reaches the platform —
		// promising otherwise would be the worst kind of interface.
		out = append(out, context_(fmt.Sprintf("<%s|Open the run>", m.Link)))
	}
	return out
}

func section(text string) any {
	return map[string]any{
		"type": "section",
		"text": map[string]any{"type": "mrkdwn", "text": text},
	}
}

func fields(texts []string) any {
	list := make([]any, 0, len(texts))
	for _, t := range texts {
		list = append(list, map[string]any{"type": "mrkdwn", "text": t})
	}
	return map[string]any{"type": "section", "fields": list}
}

func context_(text string) any {
	return map[string]any{
		"type":     "context",
		"elements": []any{map[string]any{"type": "mrkdwn", "text": text}},
	}
}

func reasonOr(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}
