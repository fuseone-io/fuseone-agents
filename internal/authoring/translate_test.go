package authoring_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// The model translates; it never grants. What comes back is read against the
// catalogue, and anything that is not in it does not exist.

var catalogue = []domain.ToolID{"crm.lookup", "kb.search", "crm.reply"}

func TestRead_toolsTheCatalogueDoesNotHave_areDropped(t *testing.T) {
	t.Parallel()

	got, err := authoring.Read([]byte(`{
	  "tools": ["crm.lookup", "crm.delete_everything"],
	  "steps": [{"name": "Identificar", "reaches": ["crm.lookup"]}]
	}`), catalogue)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A model that names a tool nobody connected has invented a capability.
	// Trusting it would make the interview a way to widen an agent's reach by
	// describing a process persuasively.
	if len(got.Tools) != 1 || got.Tools[0] != "crm.lookup" {
		t.Errorf("got %v", got.Tools)
	}
}

func TestRead_aStepReachingAnUnknownTool_losesTheTool_notTheStep(t *testing.T) {
	t.Parallel()

	got, err := authoring.Read([]byte(`{
	  "tools": ["crm.lookup"],
	  "steps": [{"name": "Resumir", "reaches": ["magic.summarise"]}]
	}`), catalogue)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// The step survives: a stage that reaches nothing is the agent thinking,
	// which is a real shape. Dropping the step instead would silently discard
	// something the author described.
	if len(got.Steps) != 1 || got.Steps[0].Name != "Resumir" || len(got.Steps[0].Reaches) != 0 {
		t.Errorf("got %+v", got.Steps)
	}
}

func TestRead_repliesThatAreNotJSON_areRefusedRatherThanGuessedAt(t *testing.T) {
	t.Parallel()

	if _, err := authoring.Read([]byte("Claro! Aqui está o agente:"), catalogue); err == nil {
		t.Error("want a refusal")
	}
}

func TestRead_prosePaddedAroundTheJSON_isStillRead(t *testing.T) {
	t.Parallel()

	// Models pad. Refusing the whole answer over a courteous sentence would
	// spend the call and throw it away.
	got, err := authoring.Read([]byte("Segue:\n```json\n{\"tools\":[\"kb.search\"]}\n```\n"), catalogue)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "kb.search" {
		t.Errorf("got %v", got.Tools)
	}
}

type fakeCompleter struct {
	// replies are answered in order, so a two-pass translation can be given a
	// different answer for each pass.
	replies []string
	reply   string
	spent   int64
	calls   int
	prompts []string
}

func (f *fakeCompleter) Complete(_ context.Context, prompt string) (model.Completion, error) {
	f.prompts = append(f.prompts, prompt)
	text := f.reply
	if f.calls < len(f.replies) {
		text = f.replies[f.calls]
	}
	f.calls++
	return model.Completion{Text: text, Cost: domain.Cost{Micros: f.spent}}, nil
}

func TestTranslate_spendPastTheDailyCeiling_isRefusedBeforeTheCall(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{reply: `{"tools":[]}`}
	_, err := authoring.Translate(t.Context(), authoring.Job{
		Completer:  fake,
		Choice:     authoring.Choice{DailyMicros: 1_000, Enabled: true},
		SpentToday: 1_000,
		Catalogue:  catalogue,
	})

	// Refused before the request leaves, not after the money is gone. A
	// ceiling checked afterwards is a report, not a ceiling.
	if !errors.Is(err, authoring.ErrOverCeiling) {
		t.Fatalf("got %v, want ErrOverCeiling", err)
	}
	if fake.calls != 0 {
		t.Errorf("the call went out anyway")
	}
}

func TestTranslate_disabledAssistant_neverCalls(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{reply: `{"tools":[]}`}
	if _, err := authoring.Translate(t.Context(), authoring.Job{
		Completer: fake,
		Choice:    authoring.Choice{DailyMicros: 1_000_000},
		Catalogue: catalogue,
	}); err == nil {
		t.Error("want a refusal")
	}
	if fake.calls != 0 {
		t.Errorf("a switched-off assistant made a call")
	}
}

func TestTranslate_reportsWhatItSpent_evenWhenTheReplyIsUnreadable(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{reply: "desculpe, não entendi", spent: 4_200}
	got, err := authoring.Translate(t.Context(), authoring.Job{
		Completer: fake,
		Choice:    authoring.Choice{DailyMicros: 1_000_000, Enabled: true},
		Catalogue: catalogue,
	})

	// The money left whether or not the answer was usable. Losing the figure
	// on the error path is how a ceiling drifts from what was actually spent.
	if err == nil {
		t.Fatal("want a refusal")
	}
	if got.Cost.Micros != 4_200 {
		t.Errorf("got %d micros, want the spend reported", got.Cost.Micros)
	}
}

func TestPrompt_asksForTheExceptionOnTheStepItBelongsTo(t *testing.T) {
	t.Parallel()

	got := authoring.Prompt(authoring.Answers{
		GoesWrong: "às vezes o cliente não está cadastrado; aí eu aviso e paro",
	}, catalogue)

	// FU-04 is answered per step, and a run that ends "the customer was not
	// found" has to be anchored where it happened. Asked for loosely, the
	// model returns four steps with an empty stops_when and the author's
	// exception is simply lost.
	if !strings.Contains(got, "stops_when") || !strings.Contains(got, "exceç") {
		t.Errorf("the prompt does not ask for the exception per step:\n%s", got)
	}
}

func TestTranslate_placesTheExceptionOnTheStepItBelongsTo(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{replies: []string{
		`{"tools":["crm.lookup"],"steps":[
		   {"name":"Identificar o cliente","reaches":["crm.lookup"]},
		   {"name":"Responder"}]}`,
		`{"step":0,"stops_when":"não encontrar o cliente"}`,
	}}

	got, err := authoring.Translate(t.Context(), authoring.Job{
		Completer: fake,
		Choice:    authoring.Choice{DailyMicros: 1_000_000, Enabled: true},
		Answers:   authoring.Answers{GoesWrong: "às vezes o cliente não está cadastrado; aviso e paro"},
		Catalogue: catalogue,
	})
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	// Asked on its own rather than as one field among many. Given as part of
	// a larger request it came back empty on every step across two live
	// attempts, and an exception attached to no step costs stage 4 its anchor.
	if got.Translated.Steps[0].StopsWhen != "não encontrar o cliente" {
		t.Errorf("got %+v", got.Translated.Steps)
	}
	if got.Translated.Steps[1].StopsWhen != "" {
		t.Errorf("the exception landed on every step: %+v", got.Translated.Steps)
	}
}

func TestTranslate_noExceptionAnswered_makesOnlyOneCall(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{reply: `{"tools":[],"steps":[{"name":"Resumir"}]}`}
	if _, err := authoring.Translate(t.Context(), authoring.Job{
		Completer: fake,
		Choice:    authoring.Choice{DailyMicros: 1_000_000, Enabled: true},
		Catalogue: catalogue,
	}); err != nil {
		t.Fatalf("translate: %v", err)
	}

	// A second call costs money and time. Nothing to place means nothing to
	// ask.
	if fake.calls != 1 {
		t.Errorf("made %d calls", fake.calls)
	}
}

func TestTranslate_secondPassNamingAStepThatIsNotThere_leavesTheStepsStanding(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{replies: []string{
		`{"tools":[],"steps":[{"name":"Resumir"}]}`,
		`{"step":7,"stops_when":"qualquer coisa"}`,
	}}

	got, err := authoring.Translate(t.Context(), authoring.Job{
		Completer: fake,
		Choice:    authoring.Choice{DailyMicros: 1_000_000, Enabled: true},
		Answers:   authoring.Answers{GoesWrong: "algo dá errado"},
		Catalogue: catalogue,
	})

	// The steps are the expensive half. Losing them because the cheap half
	// answered nonsense would spend the call and return nothing.
	if err != nil || len(got.Translated.Steps) != 1 {
		t.Fatalf("got %+v, err %v", got.Translated, err)
	}
}

func TestTranslate_bothPasses_areChargedTogether(t *testing.T) {
	t.Parallel()

	fake := &fakeCompleter{spent: 1_000, replies: []string{
		`{"tools":[],"steps":[{"name":"Resumir"}]}`,
		`{"step":0,"stops_when":"parar"}`,
	}}

	got, _ := authoring.Translate(t.Context(), authoring.Job{
		Completer: fake,
		Choice:    authoring.Choice{DailyMicros: 1_000_000, Enabled: true},
		Answers:   authoring.Answers{GoesWrong: "algo"},
		Catalogue: catalogue,
	})

	// Two calls left the installation. Reporting one is how a ceiling drifts.
	if got.Cost.Micros != 2_000 {
		t.Errorf("got %d micros, want both calls", got.Cost.Micros)
	}
}
