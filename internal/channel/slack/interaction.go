package slack

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

/*
Reading what somebody pressed.

Slack posts a form whose single field holds a JSON document, and the part that
matters is the button's own value — everything else in that document describes
the message, the workspace and the person, none of which decides anything here.

The value carries the run and the sequence, because a decision has to name the
step it answers. A button that carried only the run would let a message sitting
in a channel answer whatever the run happens to be waiting on now, which is the
stale-tab problem with a longer fuse: a conversation keeps its buttons for ever.
*/

// Interaction is one press, reduced to what a decision needs.
type Interaction struct {
	// User is the account that pressed, as the channel knows it. Never a
	// display name, which people change and which two people can share.
	User string
	// Conversation is where it was pressed, for the record.
	Conversation string

	RunID    domain.RunID
	AtSeq    int64
	Approved bool
}

// Decision encodes a button's value: what it answers, and about which step.
func Decision(run domain.RunID, atSeq int64, approved bool) string {
	verdict := "refuse"
	if approved {
		verdict = "approve"
	}
	return fmt.Sprintf("%s:%s:%d", verdict, run, atSeq)
}

// ReadInteraction pulls the press out of Slack's form-encoded payload.
func ReadInteraction(body []byte) (Interaction, error) {
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return Interaction{}, fmt.Errorf("slack: unreadable form: %w", err)
	}

	var payload struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Channel struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"channel"`
		Actions []struct {
			Value string `json:"value"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(form.Get("payload")), &payload); err != nil {
		return Interaction{}, fmt.Errorf("slack: unreadable payload: %w", err)
	}
	if len(payload.Actions) == 0 {
		return Interaction{}, fmt.Errorf("slack: the payload carries no action")
	}

	action := Interaction{
		User:         payload.User.ID,
		Conversation: conversationOf(payload.Channel.Name, payload.Channel.ID),
	}
	if err := action.readValue(payload.Actions[0].Value); err != nil {
		return Interaction{}, err
	}
	return action, nil
}

func (i *Interaction) readValue(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return fmt.Errorf("slack: unreadable action %q", value)
	}

	switch parts[0] {
	case "approve":
		i.Approved = true
	case "refuse":
		i.Approved = false
	default:
		// Refused rather than defaulted. A value nobody recognises must not
		// fall through to either answer, and the safe-looking one — refuse —
		// is a decision somebody did not take.
		return fmt.Errorf("slack: unknown verdict %q", parts[0])
	}

	seq, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return fmt.Errorf("slack: unreadable step in %q", value)
	}
	i.RunID, i.AtSeq = domain.RunID(parts[1]), seq
	return nil
}

func conversationOf(name, id string) string {
	if name != "" {
		return "#" + name
	}
	return id
}
