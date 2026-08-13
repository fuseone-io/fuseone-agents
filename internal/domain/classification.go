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

	// By and Reason record who ruled and why. Classification is the single
	// point where write access enters the system, so it is never anonymous.
	By     UserID
	Reason string
}
