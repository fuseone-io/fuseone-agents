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
	// By and Reason record who ruled and why. Classification is the single
	// point where write access enters the system, so it is never anonymous.
	By     UserID
	Reason string
}
