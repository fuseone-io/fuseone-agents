package authoring_test

import (
	"testing"

	"github.com/fuseone/agents/internal/authoring"
)

// The interview is a guided conversation filling a fixed schema, not a
// free-form prompt (PRD §6.1). The questions are the platform's and never a
// model's: an authoring path whose questions vary per run cannot be reviewed,
// reproduced or audited.

func TestQuestions_areTheSevenThePRDNames(t *testing.T) {
	t.Parallel()

	got := authoring.Questions()
	if len(got) != 7 {
		t.Fatalf("got %d questions, want 7", len(got))
	}
	// The last one is the reason the set works with this audience: somebody in
	// marketing cannot draft a security policy but answers "never send to the
	// whole base without a review" without hesitating.
	if got[6].Fills != authoring.FillsBlockingPolicy {
		t.Errorf("last question fills %s", got[6].Fills)
	}
}

func TestDraftFrom_answersThatNeedNoModel_areReadStraightThrough(t *testing.T) {
	t.Parallel()

	draft := authoring.DraftFrom(authoring.Answers{
		Name:    "Atendimento de suporte",
		Area:    "cx",
		Trigger: authoring.Trigger{Kind: "cron", Value: "*/15 * * * *"},
		Closing: "quando o cliente for respondido",
		NeverDo: "responder sem revisão",
	})

	// A schedule is a schedule. Sending it through a model to be understood
	// would be spending money to make a fact less certain.
	if len(draft.Triggers) != 1 || draft.Triggers[0].Schedule != "*/15 * * * *" {
		t.Errorf("got %+v", draft.Triggers)
	}
	if draft.Name != "Atendimento de suporte" || draft.Area != "cx" {
		t.Errorf("got %+v", draft)
	}
}

func TestDraftFrom_noTrigger_producesAnAgentSomebodyStartsByHand(t *testing.T) {
	t.Parallel()

	draft := authoring.DraftFrom(authoring.Answers{Name: "A", Area: "cx"})

	// Never a schedule nobody asked for. An agent that starts itself because
	// the wizard defaulted is the worst possible default in this product.
	if len(draft.Triggers) != 0 {
		t.Errorf("got %+v, want nothing", draft.Triggers)
	}
}

func TestDraftFrom_alwaysArrivesBounded(t *testing.T) {
	t.Parallel()

	draft := authoring.DraftFrom(authoring.Answers{Name: "A", Area: "cx"})

	// Nobody is asked "what is your cost ceiling" in the interview, and an
	// unbounded agent is one nothing obliges to finish. So the draft carries
	// a ceiling the author can raise rather than one they must invent.
	if draft.Budget.Micros <= 0 || draft.Budget.Steps <= 0 {
		t.Errorf("got %+v", draft.Budget)
	}
}
