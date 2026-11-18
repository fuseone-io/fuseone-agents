package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/authoring/conversation"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/jackc/pgx/v5"
)

/*
The conversation's routes (NT-018 phase 3, contracts 7 and 8). The engine and
the store decide the turn, against Postgres in their own tests and in the
journey; these pin what the route decides: which model, price and ceiling a
turn runs with, what the scope offers it, and how each refusal answers.
*/

func (d *draftStub) Turns(_ context.Context, _ domain.AgentID, c drafts.Caller) ([]drafts.Turn, error) {
	d.callers = append(d.callers, c)
	return d.turns, d.err
}

func (d *draftStub) BeginTurn(_ context.Context, b drafts.Beginning) (drafts.Started, error) {
	d.begun = append(d.begun, b)
	if d.beginErr != nil {
		return drafts.Started{}, d.beginErr
	}
	return drafts.Started{Turn: drafts.Turn{ID: "t-1", Key: b.Key, Read: b.Read, State: drafts.TurnInProgress, Reserved: b.Reserve}}, nil
}

func (d *draftStub) ReplayTurn(context.Context, drafts.Replaying) (drafts.Started, bool, error) {
	if d.stored == nil {
		return drafts.Started{}, false, nil
	}
	return drafts.Started{Turn: *d.stored, Replayed: true}, true, nil
}

func (d *draftStub) SettleTurn(_ context.Context, s drafts.Settlement) (drafts.Settled, error) {
	state := drafts.TurnDone
	if s.Failed {
		state = drafts.TurnFailed
	}
	out := drafts.Settled{Turn: drafts.Turn{ID: s.Turn, Key: "k1", Read: d.opened.Revision, State: state, Cost: s.Cost, Reply: s.Reply}}
	for range s.Proposed {
		out.Proposals = append(out.Proposals, drafts.Proposal{ID: "p-1", Valid: true})
	}
	return out, nil
}

func (d *draftStub) EraseThread(_ context.Context, _ domain.AgentID, c drafts.Caller) error {
	d.callers = append(d.callers, c)
	return d.err
}

// turnModelsStub is the registry: one provider configured in one area, and a
// model that answers what it is told and keeps what it was configured with.
type turnModelsStub struct {
	reply  string
	err    error
	priced bool
	asked  []model.Config
	prompt string
}

func (m *turnModelsStub) Completer(_ string, cfg model.Config) (model.Completer, error) {
	m.asked = append(m.asked, cfg)
	return m, nil
}

func (m *turnModelsStub) Complete(_ context.Context, prompt string) (model.Completion, error) {
	m.prompt = prompt
	return model.Completion{Text: m.reply, Cost: domain.Cost{Micros: 40}}, m.err
}

func (m *turnModelsStub) PriceFor(string, string) (model.Prices, bool, error) {
	return model.Prices{InputMicros: 3_000_000, OutputMicros: 15_000_000}, m.priced, nil
}

func (m *turnModelsStub) Names() []string { return []string{"anthropic", "local-llm"} }

func (m *turnModelsStub) Configured(name string, at domain.Scope) bool {
	return name == "anthropic" && at.Area == "cx"
}

var turnChoice = authoring.Choice{Provider: "anthropic", Model: "claude-x", DailyMicros: 5_000_000, Enabled: true}

func turnServer(stub *draftStub, models *turnModelsStub, choice authoring.Choice) *Server {
	tools := &fakeTools{entries: []domain.ToolEntry{
		{ID: "crm.lookup", OnSurface: true, Scope: domain.Scope{Company: "acme"}},
		{ID: "crm.archived", Scope: domain.Scope{Company: "acme"}},
		{ID: "billing.refund", OnSurface: true, Scope: domain.Scope{Company: "acme"}},
	}}
	return NewServer(ledger.NewMemory(), "test").WithDrafts(stub).
		WithAdministration(nil, tools, nil).
		WithAuthoring(interviewAuthoring{choice: choice}).
		WithTurnModels(models)
}

func sendTurn(t *testing.T, s *Server, message string) openapi.SendDraftTurnResponseObject {
	t.Helper()
	resp, err := s.SendDraftTurn(inArea("cx", domain.RoleAuthor), openapi.SendDraftTurnRequestObject{
		AgentId: "triage", Body: &openapi.DraftTurnRequest{Key: "k1", Message: message},
	})
	if err != nil {
		t.Fatalf("SendDraftTurn: %v", err)
	}
	return resp
}

func TestSendDraftTurn_runsWithTheChosenModelItsOutputCapAndTheDaysCeiling(t *testing.T) {
	t.Parallel()
	stub := &draftStub{opened: someDraft()}
	models := &turnModelsStub{reply: `{"reply":"Pronto.","proposals":[]}`, priced: true}

	resp := sendTurn(t, turnServer(stub, models, turnChoice), "Ajude.")
	got, ok := resp.(openapi.SendDraftTurn200JSONResponse)
	if !ok || got.State != openapi.DraftTurnStateDone || got.CostMicros != 40 || valueOr(got.Reply) != "Pronto." {
		t.Fatalf("response = %#v, want the turn done at its cost", resp)
	}
	if len(models.asked) != 1 || models.asked[0].Model != "claude-x" || models.asked[0].MaxTokens != conversation.MaxOutputTokens {
		t.Errorf("model configured as %+v", models.asked)
	}
	b := stub.begun[0]
	if b.DailyMicros != 5_000_000 || b.Read != `"v1-draft"` || b.Caller.By != "usr_ana" || b.Reserve <= 0 {
		t.Errorf("turn begun as %+v", b)
	}
}

// The scope offers its tools on the installation's surface, sorted so the
// same draft renders the same prompt, and only the providers configured there.
func TestSendDraftTurn_offersTheScopesToolsAndProvidersOnly(t *testing.T) {
	t.Parallel()
	models := &turnModelsStub{reply: `{"reply":"x","proposals":[]}`, priced: true}
	sendTurn(t, turnServer(&draftStub{opened: someDraft()}, models, turnChoice), "Ajude.")

	refund, lookup := strings.Index(models.prompt, `"billing.refund"`), strings.Index(models.prompt, `"crm.lookup"`)
	if refund < 0 || lookup < refund || strings.Contains(models.prompt, "crm.archived") {
		t.Errorf("tools offered out of order or off the surface:\n%s", models.prompt)
	}
	if !strings.Contains(models.prompt, `"anthropic"`) || strings.Contains(models.prompt, `"local-llm"`) {
		t.Errorf("providers offered beyond the scope's:\n%s", models.prompt)
	}
}

// A key already stored answers what was stored before anything about a new
// turn is judged: the assistant switched off since does not hide it.
func TestSendDraftTurn_aStoredKey_isAnsweredEvenWithTheAssistantOff(t *testing.T) {
	t.Parallel()
	off := turnChoice
	off.Enabled = false
	stub := &draftStub{opened: someDraft(), stored: &drafts.Turn{ID: "t-0", Key: "k1", State: drafts.TurnDone, Cost: 25}}
	models := &turnModelsStub{priced: true}

	resp := sendTurn(t, turnServer(stub, models, off), "Ajude.")
	if got, ok := resp.(openapi.SendDraftTurn200JSONResponse); !ok || got.TurnId != "t-0" || got.CostMicros != 25 {
		t.Fatalf("response = %#v, want the stored turn", resp)
	}
	if len(stub.begun) != 0 || len(models.asked) != 0 {
		t.Errorf("a stored key began %d turns and built %d models", len(stub.begun), len(models.asked))
	}
}

// A model that failed still recorded and settled the turn: answered, as
// failed, never as an error the person cannot read.
func TestSendDraftTurn_aFailedModel_answersTheTurnAsFailed(t *testing.T) {
	t.Parallel()
	models := &turnModelsStub{err: errors.New("upstream unavailable"), priced: true}
	resp := sendTurn(t, turnServer(&draftStub{opened: someDraft()}, models, turnChoice), "Ajude.")
	if got, ok := resp.(openapi.SendDraftTurn200JSONResponse); !ok || got.State != openapi.DraftTurnStateFailed || got.CostMicros != 40 {
		t.Fatalf("response = %#v, want 200 failed at its cost", resp)
	}
}

func TestSendDraftTurn_refusals_answerBeforeOrWithoutTheCall(t *testing.T) {
	t.Parallel()
	off := turnChoice
	off.Enabled = false
	for name, tc := range map[string]struct {
		choice   authoring.Choice
		priced   bool
		message  string
		getErr   error
		beginErr error
		check    func(openapi.SendDraftTurnResponseObject) bool
	}{
		"assistant off": {choice: off, priced: true, message: "x", check: is409(CodeConversationOff)},
		"no price":      {choice: turnChoice, message: "x", check: is409(CodeConversationUnpriced)},
		"over the day":  {choice: turnChoice, priced: true, message: "x", beginErr: drafts.ErrOverCeiling, check: is409(CodeConversationSpent)},
		"in flight":     {choice: turnChoice, priced: true, message: "x", beginErr: drafts.ErrTurnInFlight, check: is409(CodeTurnInFlight)},
		"empty":         {choice: turnChoice, priced: true, message: "  ", check: is400(CodeInvalidInput)},
		"too large":     {choice: turnChoice, priced: true, message: strings.Repeat("x", conversation.MaxMessageBytes+1), check: is400(CodeTurnTooLarge)},
		"unreachable":   {choice: turnChoice, priced: true, message: "x", getErr: drafts.ErrNotFound, check: is404},
		"draft moved": {choice: turnChoice, priced: true, message: "x",
			beginErr: &domain.StaleError{Current: `"v2-draft"`}, check: is412},
	} {
		stub := &draftStub{opened: someDraft(), err: tc.getErr, beginErr: tc.beginErr}
		models := &turnModelsStub{reply: `{"reply":"x","proposals":[]}`, priced: tc.priced}
		resp := sendTurn(t, turnServer(stub, models, tc.choice), tc.message)
		if !tc.check(resp) || models.prompt != "" {
			t.Errorf("%s: response = %#v, model called: %v", name, resp, models.prompt != "")
		}
	}
}

// is400 and is409 answer whether a refusal carries the code the console
// chooses its words by.
func is400(code Code) func(openapi.SendDraftTurnResponseObject) bool {
	return func(r openapi.SendDraftTurnResponseObject) bool {
		got, ok := r.(openapi.SendDraftTurn400ApplicationProblemPlusJSONResponse)
		return ok && valueOr(got.Type) == string(code)
	}
}

func is404(r openapi.SendDraftTurnResponseObject) bool {
	_, ok := r.(openapi.SendDraftTurn404ApplicationProblemPlusJSONResponse)
	return ok
}

func is409(code Code) func(openapi.SendDraftTurnResponseObject) bool {
	return func(r openapi.SendDraftTurnResponseObject) bool {
		got, ok := r.(openapi.SendDraftTurn409ApplicationProblemPlusJSONResponse)
		return ok && valueOr(got.Type) == string(code)
	}
}

func is412(r openapi.SendDraftTurnResponseObject) bool {
	_, ok := r.(openapi.SendDraftTurn412ApplicationProblemPlusJSONResponse)
	return ok
}

// An unknown turn's reservation is kept against the ceiling and said so; a
// settled turn keeps nothing.
func TestListDraftTurns_saysWhatAnUnknownTurnKeeps(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	stub := &draftStub{turns: []drafts.Turn{
		{ID: "t-1", Key: "k1", State: drafts.TurnDone, Reserved: 900, Cost: 40, Message: "Oi", Reply: "Olá", At: at},
		{ID: "t-2", Key: "k2", State: drafts.TurnUnknown, Reserved: 900, Erased: true, At: at},
	}}
	resp, err := draftServer(stub).ListDraftTurns(inArea("cx", domain.RoleAuthor), openapi.ListDraftTurnsRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("ListDraftTurns: %v", err)
	}
	items := resp.(openapi.ListDraftTurns200JSONResponse).Items
	if items[0].KeptMicros != nil || valueOr(items[0].Message) != "Oi" {
		t.Errorf("settled turn = %+v", items[0])
	}
	if valueOr(items[1].KeptMicros) != 900 || !items[1].Erased || items[1].Message != nil {
		t.Errorf("unknown turn = %+v, want 900 kept and its words erased", items[1])
	}
}

func TestEraseDraftThread_answersEachOutcome(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]string{nil: "204", drafts.ErrNotFound: "404"} {
		resp, got := draftServer(&draftStub{err: err}).EraseDraftThread(inArea("cx", domain.RoleAuthor),
			openapi.EraseDraftThreadRequestObject{AgentId: "triage"})
		_, is204 := resp.(openapi.EraseDraftThread204Response)
		_, is404 := resp.(openapi.EraseDraftThread404ApplicationProblemPlusJSONResponse)
		if got != nil || (want == "204") != is204 || (want == "404") != is404 {
			t.Errorf("erase with %v = %T (%v), want %s", err, resp, got, want)
		}
	}
}

// spendStub is what authoring already spent today.
type spendStub struct{ today int64 }

func (s spendStub) SpentToday(context.Context) (int64, error) { return s.today, nil }
func (spendStub) RecordSpend(context.Context, domain.Cost, domain.UserID) error {
	return nil
}

// The transcript says whether a turn could be sent now, and why not, so the
// console closes the conversation before the person writes into it.
func TestListDraftTurns_saysWhetherTheConversationIsOpen(t *testing.T) {
	t.Parallel()
	off := turnChoice
	off.Enabled = false
	for want, tc := range map[openapi.ListDraftTurns200JSONResponseBodyConversation]struct {
		choice authoring.Choice
		priced bool
		spent  int64
	}{
		openapi.ListDraftTurns200JSONResponseBodyConversationOpen:     {choice: turnChoice, priced: true, spent: 4_999_999},
		openapi.ListDraftTurns200JSONResponseBodyConversationOff:      {choice: off, priced: true},
		openapi.ListDraftTurns200JSONResponseBodyConversationUnpriced: {choice: turnChoice},
		openapi.ListDraftTurns200JSONResponseBodyConversationSpent:    {choice: turnChoice, priced: true, spent: 5_000_000},
	} {
		s := turnServer(&draftStub{opened: someDraft()}, &turnModelsStub{priced: tc.priced}, tc.choice).
			WithAssistants(nil, spendStub{today: tc.spent})
		resp, err := s.ListDraftTurns(inArea("cx", domain.RoleAuthor), openapi.ListDraftTurnsRequestObject{AgentId: "triage"})
		got, ok := resp.(openapi.ListDraftTurns200JSONResponse)
		if err != nil || !ok || got.Conversation != want {
			t.Errorf("%s: ListDraftTurns = %#v (%v)", want, resp, err)
		}
	}
}

// The transcript says what the day holds against its ceiling, the figures the
// console's "authoring today" reads; nothing while the assistant is off.
func TestListDraftTurns_saysWhatTheDayHoldsAgainstItsCeiling(t *testing.T) {
	t.Parallel()
	off := turnChoice
	off.Enabled = false
	on := turnServer(&draftStub{opened: someDraft()}, &turnModelsStub{priced: true}, turnChoice).WithAssistants(nil, spendStub{today: 310_000})
	resp, err := on.ListDraftTurns(inArea("cx", domain.RoleAuthor), openapi.ListDraftTurnsRequestObject{AgentId: "triage"})
	got, ok := resp.(openapi.ListDraftTurns200JSONResponse)
	if err != nil || !ok || valueOr(got.SpentMicros) != 310_000 || valueOr(got.DailyMicros) != 5_000_000 {
		t.Fatalf("ListDraftTurns = %#v (%v), want 310000 of 5000000", resp, err)
	}
	closed := turnServer(&draftStub{opened: someDraft()}, &turnModelsStub{priced: true}, off).WithAssistants(nil, spendStub{today: 310_000})
	resp, _ = closed.ListDraftTurns(inArea("cx", domain.RoleAuthor), openapi.ListDraftTurnsRequestObject{AgentId: "triage"})
	if got := resp.(openapi.ListDraftTurns200JSONResponse); got.SpentMicros != nil || got.DailyMicros != nil {
		t.Errorf("assistant off still says %v of %v", got.SpentMicros, got.DailyMicros)
	}
}

func (d *draftStub) WithinPublication(drafts.Publishing) func(context.Context, pgx.Tx, domain.VersionID) error {
	return func(context.Context, pgx.Tx, domain.VersionID) error { return d.err }
}
