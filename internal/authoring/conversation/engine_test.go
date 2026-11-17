package conversation_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/conversation"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A turn, end to end against Postgres (NT-018 phase 3): reserved before the
model is called, idempotent by its key, its proposals bound to the ETag the
model read, and its cost settled whatever the model answered.
*/

var cx = domain.Scope{Company: "acme", Area: "cx"}

// scripted is the model: it answers what the test gives and counts calls.
type scripted struct {
	reply string
	cost  int64
	err   error
	calls atomic.Int32
	// during runs while the model is "thinking", between the reservation
	// and the answer.
	during func()
}

func (s *scripted) Complete(context.Context, string) (model.Completion, error) {
	s.calls.Add(1)
	if s.during != nil {
		s.during()
	}
	return model.Completion{Text: s.reply, Cost: domain.Cost{Micros: s.cost}}, s.err
}

func caller() drafts.Caller {
	return drafts.Caller{By: "usr_ana", Allow: func(s domain.Scope) bool { return s == cx }}
}

func newDraft(t *testing.T, store *drafts.Store) (domain.AgentID, drafts.Opened) {
	t.Helper()
	agent := domain.AgentID(fmt.Sprintf("turn-%d", time.Now().UnixNano()))
	o, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, caller())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return agent, o
}

func engine(store *drafts.Store, model *scripted) conversation.Engine {
	return conversation.Engine{
		Store: store, Completer: model, Price: price, Priced: true, DailyMicros: 1_000_000_000,
		Validator: draft.Validator{Checker: publishable.Checker{}},
	}
}

func reply(text string, ops ...draft.Operation) string {
	var bodies []string
	for _, op := range ops {
		bodies = append(bodies, string(draft.Body(op)))
	}
	return fmt.Sprintf(`{"reply":%q,"proposals":[%s]}`, text, strings.Join(bodies, ","))
}

func run(t *testing.T, e conversation.Engine, agent domain.AgentID, key string) (conversation.Result, error) {
	t.Helper()
	return e.Run(t.Context(), conversation.Request{Agent: agent, Key: key, Message: "Ajude a triar chamados.",
		Locale: "pt-BR", Caller: caller(), Now: time.Now()})
}

func TestRun_reservesCallsSettlesAndProposesAgainstTheETagRead(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, o := newDraft(t, store)
	m := &scripted{reply: reply("Sugiro a ferramenta de busca.", draft.AddTool{Tool: "crm.lookup"}), cost: 70}

	got, err := run(t, engine(store, m), agent, "k1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Turn.State != drafts.TurnDone || got.Turn.Cost != 70 || got.Turn.Reserved <= 70 {
		t.Fatalf("turn %+v, want it settled at the real cost under a larger reservation", got.Turn)
	}
	if len(got.Proposals) != 1 || got.Proposals[0].Read != o.Revision || !got.Proposals[0].Valid {
		t.Fatalf("proposals %+v, want one bound to the ETag read", got.Proposals)
	}
	turns, err := store.Turns(t.Context(), agent, caller())
	if err != nil || len(turns) != 1 || turns[0].Reply != "Sugiro a ferramenta de busca." {
		t.Errorf("transcript = %+v (%v)", turns, err)
	}
}

// A key retried after its turn was stored answers what was stored, without
// calling the model again.
func TestRun_theSameKeyAgain_doesNotCallTheModelAgain(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	m := &scripted{reply: reply("Pronto."), cost: 10}
	if _, err := run(t, engine(store, m), agent, "k1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	again, err := run(t, engine(store, m), agent, "k1")
	if err != nil || !again.Replayed || m.calls.Load() != 1 {
		t.Fatalf("again = %+v (%v), %d calls; want the stored turn and one call", again, err, m.calls.Load())
	}
}

// Refused before the call: no price, an input over the per-turn ceiling, a
// day's ceiling the reservation does not fit. The model is never called and
// nothing is recorded.
func TestRun_refusedBeforeTheCall_neverCallsTheModel(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	for name, tc := range map[string]struct {
		change func(*conversation.Engine)
		want   error
	}{
		"no price":       {func(e *conversation.Engine) { e.Priced = false }, conversation.ErrNoPrice},
		"over the day":   {func(e *conversation.Engine) { e.DailyMicros = 1 }, drafts.ErrOverCeiling},
		"input too dear": {func(e *conversation.Engine) { e.Price.InputMicros = 1 << 40 }, conversation.ErrTurnTooLarge},
	} {
		m := &scripted{reply: reply("x")}
		e := engine(store, m)
		tc.change(&e)
		if _, err := run(t, e, agent, "k-"+name); !errors.Is(err, tc.want) || m.calls.Load() != 0 {
			t.Errorf("%s: Run = %v with %d calls, want %v and no call", name, err, m.calls.Load(), tc.want)
		}
	}
	if turns, _ := store.Turns(t.Context(), agent, caller()); len(turns) != 0 {
		t.Errorf("%d turns recorded by refusals", len(turns))
	}
}

// The model's failure, or an answer that cannot be read, still settles the
// turn at what it cost: the money left either way.
func TestRun_aFailedOrUnreadableAnswer_settlesAtItsCost(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	for name, m := range map[string]*scripted{
		"the model failed":     {err: errors.New("upstream unavailable"), cost: 15},
		"an unreadable answer": {reply: "só texto", cost: 25},
	} {
		got, err := run(t, engine(store, m), agent, "k-"+name)
		if err == nil || got.Turn.State != drafts.TurnFailed || got.Turn.Cost != m.cost {
			t.Errorf("%s: Run = %+v (%v), want the turn failed at %d", name, got.Turn, err, m.cost)
		}
	}
}

// A draft changed while the model was thinking: its proposals were made
// against an older draft and are dropped, counted, never recorded.
func TestRun_aDraftThatMovedDuringTheCall_dropsTheTurnsProposals(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, o := newDraft(t, store)
	m := &scripted{reply: reply("Sugiro.", draft.AddTool{Tool: "crm.lookup"}), cost: 10}
	m.during = func() {
		expect := o.Revision
		header := string(expect)
		ifm, _ := domain.ParseIfMatch(&header)
		if _, err := store.Apply(context.Background(), drafts.Application{Agent: agent, Op: draft.SetReplyLocale{Locale: "en-US"},
			Origin: draft.FromControl, Expect: ifm, Caller: caller(), Validator: draft.Validator{}, Now: time.Now()}); err != nil {
			t.Errorf("Apply during the call: %v", err)
		}
	}

	got, err := run(t, engine(store, m), agent, "k1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Proposals) != 0 || got.Stale != 1 || got.Turn.State != drafts.TurnDone {
		t.Fatalf("result %+v, want the turn done and its one proposal dropped as stale", got)
	}
}

func TestRun_aMessageTooLargeOrEmpty_isRefused(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	m := &scripted{reply: reply("x")}
	for message, want := range map[string]error{
		strings.Repeat("x", conversation.MaxMessageBytes+1): conversation.ErrTurnTooLarge,
		"   ": conversation.ErrEmptyMessage,
	} {
		_, err := engine(store, m).Run(t.Context(), conversation.Request{Agent: agent, Key: "k", Message: message,
			Caller: caller(), Now: time.Now()})
		if !errors.Is(err, want) || m.calls.Load() != 0 {
			t.Errorf("Run = %v, want %v and no call", err, want)
		}
	}
}
