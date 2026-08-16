package domain

// ToolClassification is the Curator's ruling about one tool.
//
// It lives in the domain because two packages that must not know about each
// other both speak it: the catalogue that enforces it and the administration
// that records it. Neither imports the other; they meet on this type.
type ToolClassification struct {
	Tool   ToolID
	Effect Effect
	// Untrusted marks a tool whose results carry data the platform did not
	// author. Reading one taints the run, which is what makes a later write
	// stop for a person (PRD SE-06).
	Untrusted bool
	// CompensatedBy is the tool that undoes this one (PRD SE-08).
	//
	// Ruled here, with the effect, because they are one judgement by one
	// person: what a tool does to the world and how to take it back are the
	// same question, and an author who chose their own compensation would be
	// deciding what "undone" means for a system they may not operate.
	//
	// Empty is the ordinary case and an honest one. Most writes cannot be
	// taken back — a sent email is sent — and a platform that invented a
	// compensation for one would be claiming to undo something it cannot.
	CompensatedBy ToolID

	/*
		Digest names the definition that was judged.

		A tool id is a string, and what a Curator read before saying "this only
		reads" was a description and a schema. A server may change both
		tomorrow and keep the name, and a ruling keyed by the name alone would
		carry forward onto a tool nobody has looked at — the one path by which
		an effect reaches production unjudged.

		The same shape as an approval carrying the step it approved. A decision
		that does not say what it was about is a decision about whatever is
		there now.

		Empty means a ruling recorded before this was kept, and it still
		applies: the honest reading of an absent digest is "we did not write
		down what was judged", not "it was about something else". Refusing
		those would stop every agent on an installation to add a check.
	*/
	Digest string

	// By and Reason record who ruled and why. Classification is the single
	// point where write access enters the system, so it is never anonymous.
	By     UserID
	Reason string
}
