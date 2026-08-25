package domain

// Agreement is how often people said yes to what an agent proposed.
//
// The measurement behind promotion and demotion (PRD FU-14, FU-15). It counts
// decisions a person actually made — an approval granted or refused — and
// nothing else: a run nobody was asked about says nothing about whether they
// would have agreed.
type Agreement struct {
	Agent    AgentID
	Approved int
	Refused  int
}

// VersionAgreement is the same measurement pinned to the version that asked.
//
// Promotion pages read this beside version-level simulations and spend. A
// refusal against v2 should not make v3 look less trusted, and a dashboard
// that cannot separate them teaches authors to distrust the numbers.
type VersionAgreement struct {
	Agent   AgentID
	Version VersionID

	Approved int
	Refused  int
}

// VersionGateBlocks counts the Gate decisions that stopped a version.
//
// It is deliberately coarse. The Trust Center needs to know whether this
// published definition is being stopped more often, not which arguments or
// vendor text caused it. Those stay in the run trail under the normal
// permissions and retention rules.
type VersionGateBlocks struct {
	Agent   AgentID
	Version VersionID

	Blocks int
	Runs   int
}

func (a Agreement) Decided() int { return a.Approved + a.Refused }

// Rate is the share people agreed with, or zero when nobody was asked.
func (a Agreement) Rate() float64 {
	if a.Decided() == 0 {
		return 0
	}
	return float64(a.Approved) / float64(a.Decided())
}

func (a VersionAgreement) Decided() int { return a.Approved + a.Refused }

func (a VersionAgreement) Rate() float64 {
	if a.Decided() == 0 {
		return 0
	}
	return float64(a.Approved) / float64(a.Decided())
}

// Thresholds for moving between stages.
//
// The floor on how many decisions is the important half. A rate over three
// approvals is not evidence of anything, and suggesting promotion on it would
// train people to dismiss the suggestion — which costs more than never making
// it.
const (
	// PromoteAfter is the fewest decisions worth reading a rate from.
	PromoteAfter = 20
	// PromoteAbove is the agreement at which promotion is worth suggesting.
	PromoteAbove = 0.95
	// DemoteAfter is how few decisions it takes to act on disagreement.
	//
	// Far lower than PromoteAfter on purpose: the two are not symmetric.
	// Loosening on thin evidence risks harm; tightening on thin evidence
	// costs somebody a few clicks, and waiting for certainty while an agent
	// is being overruled is the expensive mistake.
	DemoteAfter = 5
	// DemoteBelow is the agreement under which an agent stops acting alone.
	DemoteBelow = 0.8
)

// SuggestsPromotion reports whether people have agreed often enough, for long
// enough, that somebody should be asked about promotion.
//
// It only ever suggests. Promotion is a person's decision — an agent that
// promoted itself for agreeing with people is an agent that stops being asked
// about (FU-14).
func (a Agreement) SuggestsPromotion() bool {
	return a.Decided() >= PromoteAfter && a.Rate() >= PromoteAbove
}

// WarrantsDemotion reports whether an agent is being overruled often enough to
// stop acting alone.
//
// This one is not a suggestion. An agent whose decisions people keep refusing
// is doing damage between the moment somebody notices and the moment they get
// round to it (FU-15).
func (a Agreement) WarrantsDemotion() bool {
	return a.Decided() >= DemoteAfter && a.Rate() < DemoteBelow
}
