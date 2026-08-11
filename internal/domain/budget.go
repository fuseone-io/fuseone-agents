package domain

import "time"

// Period is the window a scope's ceiling applies over.
//
// A scope ceiling is a spending limit over time — "this area may spend R$500
// a month" — which is a different thing from the per-run ceiling in an agent's
// specification. Both are enforced by the same check; only their windows
// differ, and confusing them is how a monthly cap ends up cutting off the
// first run of the month.
type Period string

const (
	PeriodDaily   Period = "daily"
	PeriodMonthly Period = "monthly"
)

func (p Period) Valid() bool { return p == PeriodDaily || p == PeriodMonthly }

// Since returns when the current window opened, in UTC.
func (p Period) Since(now time.Time) time.Time {
	now = now.UTC()
	if p == PeriodDaily {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// ScopeBudget is a ceiling somebody configured for a scope.
type ScopeBudget struct {
	// ScopeKind is installation, company or area. It is kept alongside Scope
	// because an installation-wide budget has an empty scope, and an empty
	// scope is also what a malformed row looks like.
	ScopeKind string
	Scope     Scope

	Period  Period
	Budget  Budget
	Enabled bool

	UpdatedBy UserID
	UpdatedAt time.Time
}

// Headroom is what remains of a ceiling after what a scope already spent.
//
// Zero in a dimension means no ceiling, which is why an exhausted ceiling
// reports one micro rather than zero: reporting zero would read as "no limit"
// and let the run through.
func Headroom(ceiling Budget, spent Consumption) Budget {
	return Budget{
		Micros:      remaining(ceiling.Micros, spent.Micros),
		Tokens:      remaining(ceiling.Tokens, spent.Tokens),
		ToolCalls:   remaining(ceiling.ToolCalls, spent.ToolCalls),
		Steps:       remaining(ceiling.Steps, spent.Steps),
		WallClockMS: remaining(ceiling.WallClockMS, spent.WallClockMS),
	}
}

func remaining(ceiling, spent int64) int64 {
	if ceiling <= 0 {
		return 0 // no ceiling in this dimension
	}
	if left := ceiling - spent; left > 0 {
		return left
	}
	// Exhausted. One rather than zero, because zero means "unlimited" and
	// would turn a spent budget into a free pass.
	return 1
}
