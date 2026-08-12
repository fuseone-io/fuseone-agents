package authoring

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
The interview: seven questions filling a fixed schema.

PRD §6.1 is explicit that this is a guided conversation and not a free-form
prompt, and that settles a design question before it is asked. The questions
are the platform's, fixed and in order; no model chooses what to ask next. An
authoring path whose questions vary per run cannot be reviewed, reproduced or
audited, and this one appears at publication.

The model's job is translation, not conversation: turning a business-language
answer into specification fields, and choosing among the tools already
connected. Everything a person answered as a fact — a schedule, an area, a name
— is read straight through. Sending a cron expression to a model to be
understood would be spending money to make a certainty less certain.
*/

// Fills names the part of the specification a question populates.
type Fills string

const (
	FillsTrigger        Fills = "trigger"
	FillsReadTools      Fills = "read_tools"
	FillsSteps          Fills = "steps"
	FillsExceptions     Fills = "exceptions"
	FillsApprovals      Fills = "approvals"
	FillsClosing        Fills = "closing"
	FillsBlockingPolicy Fills = "blocking_policy"
)

// Question is one thing the author is asked, in their language.
type Question struct {
	Fills Fills
	// Key is the catalogue key of the question itself. The words live with the
	// other user-facing strings rather than in the domain.
	Key string
	// Translated reports whether a model reads this answer. Where it is false
	// the answer is a fact and is stored as given.
	Translated bool
}

// Questions is the fixed schema, in the order the PRD asks them.
func Questions() []Question {
	return []Question{
		{Fills: FillsTrigger, Key: "interview.whenDoesItStart"},
		{Fills: FillsReadTools, Key: "interview.whatMustYouKnow", Translated: true},
		{Fills: FillsSteps, Key: "interview.whatAreTheSteps", Translated: true},
		{Fills: FillsExceptions, Key: "interview.whatGoesWrong", Translated: true},
		{Fills: FillsApprovals, Key: "interview.whatWouldYouNotDecide", Translated: true},
		{Fills: FillsClosing, Key: "interview.howDoYouKnowItIsDone"},
		// Last, and the reason the set works with this audience: somebody in
		// marketing cannot draft a security policy, but answers "never send to
		// the whole base without a review" without hesitating. That is a
		// guardrail in business language (PRD FU-07).
		{Fills: FillsBlockingPolicy, Key: "interview.whatMustNeverHappen"},
	}
}

// Trigger is what the author said opens a run, before it is a spec trigger.
type Trigger struct {
	Kind  string
	Value string
}

// Answers is what the interview collected.
type Answers struct {
	Name string
	Area string

	Trigger   Trigger
	MustKnow  string
	Steps     string
	GoesWrong string
	NotDecide string
	Closing   string
	NeverDo   string
}

// Defaults an author can raise rather than one they have to invent. Nobody is
// asked for a ceiling, and an agent with none is one nothing obliges to
// finish.
const (
	defaultMicros int64 = 500_000
	defaultSteps  int64 = 60
)

// DraftFrom builds what the answers say on their own.
//
// This is the half that needs no model. What is left — which tools serve "what
// must you know", what the steps are — is translation, and it is done
// separately so that a failure there leaves a draft rather than nothing.
func DraftFrom(a Answers) spec.Spec {
	draft := spec.Spec{
		Name: a.Name,
		Area: domain.AreaID(a.Area),
		Budget: domain.Budget{
			Micros: defaultMicros,
			Steps:  defaultSteps,
		},
	}

	// Never a schedule nobody asked for. An agent that starts itself because a
	// wizard defaulted is the worst default this product could have.
	switch a.Trigger.Kind {
	case "cron":
		draft.Triggers = []spec.Trigger{{Type: "cron", Schedule: a.Trigger.Value}}
	case "webhook":
		draft.Triggers = []spec.Trigger{{Type: "webhook", Path: a.Trigger.Value}}
	case "event":
		draft.Triggers = []spec.Trigger{{Type: "event", Event: a.Trigger.Value}}
	}

	return draft
}
