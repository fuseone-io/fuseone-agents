package channel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

func TestTicketOutcomeConsumer_retriesTheSafeAnswerUntilSlackReceivedIt(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"status":"accepted"}`)
	content := engine.NewMemoryContent()
	ref, err := content.Put(t.Context(), "run-ticket", 9, raw)
	if err != nil {
		t.Fatal(err)
	}
	notice := ticket.OutcomeNotice{
		Ref:    domain.TicketRef{Key: "ticket-1", Revision: 2},
		Origin: ticket.Origin{Connection: "slack-main", Conversation: "C-support", Root: "171.1"},
		Phase:  ticket.PhaseCompleted,
		Result: ticket.ContentRef{Ref: ref, Digest: engine.ResultDigest(raw)},
	}
	store := &outcomeNoticeStub{notice: notice}
	answers := &ticketAnswers{fail: true}
	consumer := channel.NewTicketOutcomeConsumer(
		store, content, answers, fixedTicketRenderer{}, "worker-1",
	)

	if said, err := consumer.Sweep(t.Context(), time.Minute, 10); err == nil || said != 0 || store.marked {
		t.Fatalf("failed delivery = (%d, %v), marked=%v", said, err, store.marked)
	}
	answers.fail = false
	if said, err := consumer.Sweep(t.Context(), time.Minute, 10); err != nil || said != 1 || !store.marked {
		t.Fatalf("retried delivery = (%d, %v), marked=%v", said, err, store.marked)
	}
	if answers.channel != "slack-main" || answers.conversation != "C-support" ||
		answers.thread != "171.1" || answers.text != "approved safely" {
		t.Fatalf("answer = %+v", answers)
	}
}

func TestTicketOutcomeConsumer_aChangedProjectionNeverReachesSlack(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	ref, err := content.Put(t.Context(), "run-ticket", 9, []byte(`{"apiKey":"secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	store := &outcomeNoticeStub{notice: ticket.OutcomeNotice{
		Ref:    domain.TicketRef{Key: "ticket-1", Revision: 1},
		Origin: ticket.Origin{Connection: "slack-main", Conversation: "C-support", Root: "171.1"},
		Phase:  ticket.PhaseCompleted,
		Result: ticket.ContentRef{Ref: ref, Digest: engine.ResultDigest([]byte(`{"status":"accepted"}`))},
	}}
	answers := &ticketAnswers{}
	consumer := channel.NewTicketOutcomeConsumer(store, content, answers, fixedTicketRenderer{}, "worker-1")
	if said, err := consumer.Sweep(t.Context(), time.Minute, 10); err == nil || said != 0 {
		t.Fatalf("changed projection = (%d, %v), want a closed failure", said, err)
	}
	if answers.calls != 0 || store.marked {
		t.Fatalf("changed projection was delivered=%d or marked=%v", answers.calls, store.marked)
	}
}

type outcomeNoticeStub struct {
	notice ticket.OutcomeNotice
	marked bool
}

func (s *outcomeNoticeStub) ClaimOutcomes(
	context.Context, string, time.Time, time.Duration, int,
) ([]ticket.OutcomeNotice, error) {
	if s.marked {
		return nil, nil
	}
	return []ticket.OutcomeNotice{s.notice}, nil
}

func (s *outcomeNoticeStub) MarkOutcomeAnnounced(
	_ context.Context, ref domain.TicketRef, owner string, _ time.Time,
) error {
	if ref != s.notice.Ref || owner != "worker-1" {
		return ticket.ErrMoved
	}
	s.marked = true
	return nil
}

type fixedTicketRenderer struct{}

func (fixedTicketRenderer) RenderTicketOutcome(ticket.Phase, []byte) (string, error) {
	return "approved safely", nil
}

type ticketAnswers struct {
	fail                                bool
	calls                               int
	channel, conversation, thread, text string
}

func (a *ticketAnswers) Reply(context.Context, string, string, string, string) error {
	return errors.New("unexpected refusal reply")
}

func (a *ticketAnswers) ReplyOutcome(
	_ context.Context, channelName, conversation, thread, text string,
) error {
	a.calls++
	if a.fail {
		return errors.New("slack unavailable")
	}
	a.channel, a.conversation, a.thread, a.text = channelName, conversation, thread, text
	return nil
}
