package channel

// The announcement that carries the run's own answer: how the closing text
// and its declared headline are resolved, bounded, and posted. Split from the
// reporter's sweep mechanics — this file is about what an announcement says,
// the reporter about when one is owed.

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func (r *Reporter) message(ctx context.Context, report Report) Message {
	m := Message{
		Event: report.Event, RunID: report.RunID, Agent: report.AgentID,
		Scope: report.Scope, Reason: report.Reason, Tool: report.Tool,
		AtSeq: report.AtSeq, AwaitingDecision: report.AwaitingDecision,
	}
	if r.baseURL != "" {
		m.Link = fmt.Sprintf("%s/runs/%s", r.baseURL, report.RunID)
	}
	m.Answer, m.Headline = r.answerFor(ctx, report)
	return m
}

/*
answerFor is the run's own closing text, when this announcement should carry it.

Only a finished run, and only one nobody asked for: the ask path already says
the answer in the thread that asked, and carrying it here as well would say
everything twice. A reporter without the outcome ports announces the card it
always announced, which is what an installation on an older worker gets.

A failure to read the outcome degrades to the card rather than blocking the
announcement: the person still learns the run finished, and the link still
leads to the whole answer. Erasure is the exception with words of its own —
the sentence the ask path already uses — because "the platform removed this"
and "nothing was said" must not read the same.
*/
func (r *Reporter) answerFor(ctx context.Context, report Report) (answer, headline string) {
	if report.Event != EventFinished || report.Asked || r.outcomes == nil {
		return "", ""
	}
	payload, err := r.outcomes.FinishedOutcome(ctx, report.RunID)
	if err != nil {
		return "", ""
	}
	text, err := engine.OutcomeOf(ctx, r.content, payload)
	if err != nil {
		if errors.Is(err, domain.ErrContentErased) {
			return erasedAnswer, ""
		}
		return "", ""
	}
	// A headline that cannot be read degrades to the single post, never to a
	// lost answer: the line is presentation, the answer is the message.
	headline, err = engine.HeadlineOf(ctx, r.content, payload)
	if err != nil {
		headline = ""
	}
	return truncateAnswer(text), truncateHeadline(headline)
}

/*
postHeadlined is the two-post announcement: the run declared a headline for
its answer, so the headline is the channel message and the answer hangs in
its thread.

Both land before the delivery is recorded — if the reply fails after the
headline posted, the announcement stays owed and the retry may repeat the
one-liner. Chosen deliberately: a repeated line is noise, a report that never
arrives is a failure of the one promise this path makes (#165).
*/
func (r *Reporter) postHeadlined(
	ctx context.Context, owed Announcement, place Conversation, msg Message,
) (bool, error) {
	header := msg
	header.Answer, header.Headline = msg.Headline, ""
	at, err := placed(ctx, r.poster, place, header)
	if err != nil {
		return false, fmt.Errorf("channel: post to %s: %w", place.Label, err)
	}
	body := msg
	body.Headline = ""
	threaded := place
	threaded.Thread = at.Ref
	if _, err := placed(ctx, r.poster, threaded, body); err != nil {
		return false, fmt.Errorf("channel: thread answer to %s: %w", place.Label, err)
	}
	return true, r.deliveries.Record(ctx, Delivery{
		Announcement: owed, Ref: at.Ref, Placed: at.Conversation, PostedAt: r.clock(),
	})
}

// MaxAnnouncedHeadline bounds the one line an announcement opens with. The
// finish guard caps what new runs store, but a line written by anything older
// or other arrives here unbounded — and a channel refusing an oversized block
// would leave the announcement owed and retried for ever.
const MaxAnnouncedHeadline = 300

func truncateHeadline(line string) string {
	if len(line) <= MaxAnnouncedHeadline {
		return line
	}
	cut := MaxAnnouncedHeadline
	// Never split a rune: a cut through a multibyte character renders as
	// garbage in the one place a person reads.
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut]
}

// MaxAnnouncedAnswer bounds what an announcement carries. Slack refuses a
// section past its own 3000-character limit, so an uncut answer would not be a
// long message — it would be no message, failing every hour for as long as the
// answer stays long. The margin below the vendor limit leaves room for the
// truncation notice and mrkdwn escaping.
const MaxAnnouncedAnswer = 2800

const erasedAnswer = "The agent finished, but its closing answer was erased by retention or a data erasure request."

const truncationNotice = "\n\n_(truncated — the full answer is in the console)_"

func truncateAnswer(text string) string {
	if len(text) <= MaxAnnouncedAnswer {
		return text
	}
	cut := MaxAnnouncedAnswer - len(truncationNotice)
	// Never split a rune: a cut through a multibyte character renders as
	// garbage in the one place a person reads.
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + truncationNotice
}
