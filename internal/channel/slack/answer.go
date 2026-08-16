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

	AnswerUnknown Answer = "This account is linked to somebody the directory no longer has."

	/*
		AnswerUnverified is a lookup that failed, and it is not any of the
		above.

		Slack replaces the original message with this, so it is both the
		diagnosis and the last thing the reader is told. Answered with
		AnswerUnknown, a store that was away tells somebody their account
		points at a person who no longer exists — a specific claim the failure
		did not prove, about a state that is probably not true. Answered with
		AnswerUnbound it is worse, because they would go and have an
		administrator link an account that is already linked.

		The one honest sentence here is that we could not tell, and that trying
		again is worth doing — which none of the others say, because none of
		them is a state you can retry out of.
	*/
	AnswerUnverified Answer = "Your linked account could not be checked just now. " +
		"Nothing was decided — try again in a moment."
	AnswerForbidden Answer = "You do not hold approval in this run's area."
	AnswerDecided   Answer = "Already decided — somebody answered this one first."
	AnswerGone      Answer = "That run is no longer waiting on a decision."
	AnswerFailed    Answer = "The decision could not be recorded. Nothing changed."
)
