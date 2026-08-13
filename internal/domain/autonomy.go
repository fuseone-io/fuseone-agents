package domain

// Stage is how much an agent is trusted to do on its own (PRD FU-14, FU-15).
//
// State beside the specification rather than a field in it, for the reason the
// pause flag is: a published version is immutable and every run is pinned to
// one, and promotion is not a new version. An agent promoted on a Tuesday is
// the same agent doing the same thing, trusted further.
type Stage string

const (
	// StageDraft cannot open a real run at all. It can be simulated, which is
	// how it earns its way out (FU-10).
	StageDraft Stage = "draft"
	// StageCopilot acts only with a person's approval, whatever the policy
	// would otherwise allow. It is the stage where an installation finds out
	// whether it agrees with the agent, on real work, at no risk.
	StageCopilot Stage = "copilot"
	// StageAutonomous is judged by the policy set like anything else.
	StageAutonomous Stage = "autonomous"
)

func (s Stage) Valid() bool {
	switch s {
	case StageDraft, StageCopilot, StageAutonomous:
		return true
	}
	return false
}

// StageOf reads a stage, defaulting an unset one to draft.
//
// Draft rather than autonomous, because the default is what an agent gets when
// nobody has decided — and nobody deciding is the least of all reasons to let
// something act unsupervised.
func StageOf(s string) Stage {
	if stage := Stage(s); stage.Valid() {
		return stage
	}
	return StageDraft
}

// NeedsApproval reports whether this stage requires a person for an effect.
//
// Reads pass: a copilot that asked permission to look something up would be
// asking a person to click through the whole run, and a person clicking
// through is a person not reading.
func (s Stage) NeedsApproval(effect Effect) bool {
	return s == StageCopilot && effect != EffectRead
}
