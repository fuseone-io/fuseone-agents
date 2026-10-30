package domain

// ModelPair is a provider and one of its models: the unit a planning call is
// addressed to and billed under.
type ModelPair struct {
	Provider string `json:"provider" yaml:"provider,omitempty"`
	Model    string `json:"model" yaml:"model,omitempty"`
}

// IsZero reports a pair nobody declared.
func (p ModelPair) IsZero() bool { return p.Provider == "" && p.Model == "" }

// ParkFallbackUnrehearsed is why an autonomous run that would have switched
// to its fallback parks instead.
//
// Its own code rather than the provider's failure code. The provider's code
// says what broke; this one says what the platform declined to do about it —
// an autonomous agent was promoted on evidence gathered against its declared
// pair, and nothing has rehearsed it against the other one yet.
const ParkFallbackUnrehearsed = "fallback_unrehearsed"

/*
ModelFallbackPayload records that a run stopped asking its declared pair and
asks the fallback from here on.

A step of its own, because the switch is a state transition the fold has to
see: any worker may pick the run up next, and it must plan with the fallback
without anybody's memory of the outage. The failure is the stable summary of
the one that exhausted the attempts, never the provider's text (AU-04).
*/
type ModelFallbackPayload struct {
	From     ModelPair      `json:"from"`
	To       ModelPair      `json:"to"`
	Failure  FailureSummary `json:"failure"`
	Attempts int            `json:"attempts,omitempty"`
}
