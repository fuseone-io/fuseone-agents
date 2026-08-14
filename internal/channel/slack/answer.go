package slack

// Answer is what the message becomes once somebody has pressed a button.
//
// Slack replaces the original with whatever comes back, which is how the
// buttons stop being pressable — and it is also the only place a reader learns
// what happened, so each of these has to be a sentence and not a code.
//
// English, like the rest of what this driver posts: a conversation has many
// readers and no session, so there is no locale to render in.
type Answer string

const (
	AnswerApproved Answer = "Approved. The run is continuing."
	AnswerRefused  Answer = "Refused. The run will not take that action."

	// AnswerUnbound is the commonest refusal and the one worth being exact
	// about: the person is real, the platform simply does not know that this
	// account is them. "Something went wrong" would send them to debug a
	// platform working exactly as intended.
	AnswerUnbound Answer = "This account is not linked to anybody in FuseOne Agents. " +
		"An administrator has to link it before you can decide here."

	AnswerUnknown   Answer = "This account is linked to somebody the directory no longer has."
	AnswerForbidden Answer = "You do not hold approval in this run's area."
	AnswerDecided   Answer = "Already decided — somebody answered this one first."
	AnswerGone      Answer = "That run is no longer waiting on a decision."
	AnswerFailed    Answer = "The decision could not be recorded. Nothing changed."
)
