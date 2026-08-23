package model

import "testing"

// A step that names its own model is priced as that model.
//
// The planner is built for the agent's model and carries its rate. Before this
// it billed every call at that rate, so a step switching to a cheaper model
// was recorded at the expensive one — wrong in a direction nobody notices,
// which is the whole reason the aggregate is per planning call.
func TestRateFor_stepModelIsPricedAsItself(t *testing.T) {
	t.Parallel()

	a := &Anthropic{cfg: Config{
		Model:           "claude-opus-5",
		PricePerMTok:    Prices{InputMicros: 5_000_000},
		PriceConfigured: true,
		RateFor: func(model string) (Prices, bool) {
			if model == "claude-haiku-4-5" {
				return Prices{InputMicros: 1_000_000}, true
			}
			return Prices{}, false
		},
	}}

	rate, configured := a.rateFor("claude-haiku-4-5")
	if !configured || rate.InputMicros != 1_000_000 {
		t.Fatalf("step model priced at %d, want its own rate", rate.InputMicros)
	}

	// The planner's own model still resolves through the same path.
	base, ok := a.rateFor("claude-opus-5")
	if !ok || base.InputMicros != 5_000_000 {
		t.Errorf("base model priced at %d, want the planner's rate", base.InputMicros)
	}
}
