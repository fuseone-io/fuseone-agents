package channel_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
Turning an ask into a run.

Scope before agent, agent before ask, ask before run. Each step bounds what the
next may do, and none consults the text for anything the platform decides: the
scope comes from the conversation, never from what somebody wrote in it.

Every refusal is answered where the ask was made. A channel that goes quiet is
indistinguishable from a channel that is broken, and the second time somebody
is ignored they stop asking.
*/

func TestSweep_anAskInAMappedConversation_opensARun(t *testing.T) {
	c, parts := consumerFor(t, "<@U07BOT> triagem esse chamado é sobre boleto")

	opened, err := c.Sweep(t.Context(), time.Minute, 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 1 {
		t.Fatalf("opened = %d, want the one ask", opened)
	}

	req := parts.opener.last
	if req.Agent != "triagem" {
		t.Errorf("agent = %q, want triagem", req.Agent)
	}
	// Somebody outside the platform typed this. On an internal channel they
	// are a colleague and the text is still theirs, and the taint check is
	// what stands between a sentence and an effect.
	if !req.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the ask marked untrusted", req.Labels)
	}
	// The reply belongs to the message that asked, and nowhere else is a
	// recipient the platform would be choosing.
	if req.Origin == nil || req.Origin.Conversation != "C07-ops" {
		t.Errorf("origin = %+v, want the conversation sealed", req.Origin)
	}
	if req.By != "usr_ana" {
		t.Errorf("on behalf of %q, want the person who asked", req.By)
	}
}

// The ledger holds the structured ask and the sentence, never the sentence
// alone: a run explained a year later as "somebody typed this in a thread" is
// a screenshot.
func TestSweep_theInput_carriesTheAskAndWhoMadeIt(t *testing.T) {
	c, parts := consumerFor(t, "<@U07BOT> triagem esse chamado é sobre boleto")

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	var ask struct {
		Text    string `json:"text"`
		AskedBy string `json:"asked_by"`
	}
	if err := json.Unmarshal(parts.opener.last.Input, &ask); err != nil {
		t.Fatalf("the input is not the structured ask: %v", err)
	}
	if ask.Text != "esse chamado é sobre boleto" {
		t.Errorf("text = %q, want the person's own words", ask.Text)
	}
	if ask.AskedBy != "usr_ana" {
		t.Errorf("asked by %q, want the resolved principal", ask.AskedBy)
	}
}

func TestSweep_anAskNamingNoAgent_isAnsweredInTheConversation(t *testing.T) {
	c, parts := consumerFor(t, "<@U07BOT> alguém aí?")

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if parts.opener.calls != 0 {
		t.Error("a run opened for an ask that named no agent")
	}
	if len(parts.answers.said) != 1 {
		t.Fatalf("said = %v, want one answer", parts.answers.said)
	}
	// The refusal teaches. Nobody reads documentation before typing in a chat.
	if !strings.Contains(parts.answers.said[0], "triagem") {
		t.Errorf("answer = %q, want it to name what is startable", parts.answers.said[0])
	}
}

/*
An account nobody bound speaks for nobody.

A run acts on somebody's behalf, and running as nobody is how an ask acquires
authority no person holds. Refused by name: "something went wrong" would send
an operator to debug a platform working exactly as intended.
*/
func TestSweep_anUnboundAccount_opensNothingAndSaysWhy(t *testing.T) {
	c, parts := consumerFor(t, "<@U07BOT> triagem algo")
	parts.bound = false

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if parts.opener.calls != 0 {
		t.Error("a run opened on behalf of nobody")
	}
	if len(parts.answers.said) != 1 || !strings.Contains(parts.answers.said[0], "not linked") {
		t.Errorf("said = %v, want the reason named", parts.answers.said)
	}
}

// A conversation nobody mapped starts nothing, and the person is told it is an
// operator's job rather than left wondering.
func TestSweep_aConversationSpeakingForNoScope_saysSo(t *testing.T) {
	c, parts := consumerFor(t, "<@U07BOT> triagem algo")
	parts.scopes.err = errors.New("no conversation by that id")

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if parts.opener.calls != 0 {
		t.Error("a run opened from an unmapped conversation")
	}
	if len(parts.answers.said) != 1 {
		t.Errorf("said = %v, want the person told", parts.answers.said)
	}
}

// An agent that never declared willingness is not startable, however the
// conversations are mapped.
func TestSweep_anAgentThatNeverDeclaredIt_isNotStartable(t *testing.T) {
	c, parts := consumerFor(t, "<@U07BOT> triagem algo")
	parts.willing = false

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if parts.opener.calls != 0 {
		t.Error("an agent that declared nothing was started by a message")
	}
}

/*
A reply inside a thread the platform itself posted resolves to that run.

The boundary of what the platform is entitled to claim it knows. Anything else
stays absent, which leaves the agent to go and search — a tool call somebody
can audit rather than a guess this edge made silently.
*/
func TestSweep_aReplyInOurOwnThread_carriesTheSubject(t *testing.T) {
	c, parts := consumerWith(t, "<@U07BOT> triagem investiga isso", func(p *consumerParts) {
		p.arrival.Thread = "1786.1"
		p.subjects.run = "run-alerta"
	})

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	var ask struct {
		Subject *struct {
			Run string `json:"run"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(parts.opener.last.Input, &ask); err != nil {
		t.Fatalf("input: %v", err)
	}
	if ask.Subject == nil || ask.Subject.Run != "run-alerta" {
		t.Errorf("subject = %+v, want the run the thread is about", ask.Subject)
	}
}

func TestSweep_aReplyInSomebodyElsesThread_carriesNoSubject(t *testing.T) {
	c, parts := consumerWith(t, "<@U07BOT> triagem investiga isso", func(p *consumerParts) {
		p.arrival.Thread = "9999.9"
		p.subjects.found = false
	})

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if strings.Contains(string(parts.opener.last.Input), "subject") {
		t.Errorf("input = %s, want no subject invented", parts.opener.last.Input)
	}
}

// --- the harness ------------------------------------------------------------

type consumerParts struct {
	inbox    *channel.Inbox
	arrival  channel.Arrival
	scopes   *scopeStub
	subjects *subjectStub
	opener   *openerSpy
	answers  *answerSpy
	bound    bool
	willing  bool
}

func consumerFor(t *testing.T, said string) (*channel.Consumer, *consumerParts) {
	return consumerWith(t, said, nil)
}

// consumerWith lets a test change the arrival before it is written down. The
// inbox refuses a second delivery of the same event, which is the whole point
// of it — so a helper that seeded first and mutated after would be testing the
// row it did not change.
func consumerWith(
	t *testing.T, said string, tweak func(*consumerParts),
) (*channel.Consumer, *consumerParts) {
	t.Helper()
	p := &consumerParts{
		inbox:    freshInbox(t),
		scopes:   &scopeStub{scope: domain.Scope{Company: "acme", Area: "ops"}},
		subjects: &subjectStub{found: true},
		opener:   &openerSpy{},
		answers:  &answerSpy{},
		bound:    true,
		willing:  true,
		arrival: channel.Arrival{
			Channel: "acme-slack", Conversation: "C07-ops", EventID: "Ev1",
			AskedBy: "U9", Text: said, Thread: "Ev1",
			Payload: []byte(`{}`),
		},
	}
	if tweak != nil {
		tweak(p)
	}
	reseed(t, p)

	c := channel.NewConsumer(p.inbox, "worker-1", slog.New(slog.NewTextHandler(io.Discard, nil))).
		With(p.scopes, p, p, p.subjects, p.opener, p.answers).
		Binding(func(_ context.Context, _, _ string) (domain.UserID, bool) {
			return "usr_ana", p.bound
		})
	return c, p
}

func reseed(t *testing.T, p *consumerParts) {
	t.Helper()
	if _, err := p.inbox.Receive(t.Context(), p.arrival); err != nil {
		t.Fatalf("Receive: %v", err)
	}
}

func (p *consumerParts) List(context.Context, domain.Scope, bool) ([]domain.AgentSummary, error) {
	return []domain.AgentSummary{
		{ID: "triagem", VersionID: "v1", Name: "Triagem de chamados"},
	}, nil
}

func (p *consumerParts) StartableFromConversation(
	context.Context, domain.AgentID, domain.VersionID,
) (bool, error) {
	return p.willing, nil
}

type scopeStub struct {
	scope domain.Scope
	err   error
}

func (s *scopeStub) ScopeOf(context.Context, string, string) (domain.Scope, error) {
	return s.scope, s.err
}

type subjectStub struct {
	run   domain.RunID
	found bool
}

func (s *subjectStub) AboutRun(context.Context, string, string, string) (domain.RunID, bool, error) {
	return s.run, s.found && s.run != "", nil
}

type openerSpy struct {
	calls int
	last  channel.Request
}

func (o *openerSpy) Open(_ context.Context, req channel.Request) (channel.Opened, error) {
	o.calls++
	o.last = req
	return channel.Opened{RunID: "run_new", Created: true}, nil
}

type answerSpy struct{ said []string }

func (a *answerSpy) Reply(_ context.Context, _, _, _, text string) error {
	a.said = append(a.said, text)
	return nil
}
