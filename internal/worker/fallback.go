package worker

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// Fallback is the pair a version declared for when its own keeps failing.
type Fallback struct {
	// From is the declared pair, To the fallback, both as the author
	// reviewed them.
	From, To domain.ModelPair
	// Planner plans with To. It is handed over on every turn and asked only
	// once the run's ledger records the switch.
	Planner engine.Planner
}
