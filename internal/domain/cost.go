package domain

// Cost is the consumption recorded on a single step (PRD FO-08).
//
// Cache tokens are tracked separately on purpose: a cache read costs a
// fraction of an input token, and without the split there is no way to
// diagnose why an agent is expensive.
//
// Money is an integer in millionths of the installation's currency. Floating
// point accumulates error across thousands of steps and makes the monthly
// close fail to reconcile.
type Cost struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	Micros           int64
}

func (c Cost) Add(o Cost) Cost {
	return Cost{
		InputTokens:      c.InputTokens + o.InputTokens,
		OutputTokens:     c.OutputTokens + o.OutputTokens,
		CacheReadTokens:  c.CacheReadTokens + o.CacheReadTokens,
		CacheWriteTokens: c.CacheWriteTokens + o.CacheWriteTokens,
		Micros:           c.Micros + o.Micros,
	}
}

func (c Cost) Sub(o Cost) Cost {
	return Cost{
		InputTokens:      c.InputTokens - o.InputTokens,
		OutputTokens:     c.OutputTokens - o.OutputTokens,
		CacheReadTokens:  c.CacheReadTokens - o.CacheReadTokens,
		CacheWriteTokens: c.CacheWriteTokens - o.CacheWriteTokens,
		Micros:           c.Micros - o.Micros,
	}
}

func (c Cost) IsZero() bool {
	return c == Cost{}
}

// TotalTokens sums every class. Use it for token ceilings, not for money.
func (c Cost) TotalTokens() int64 {
	return c.InputTokens + c.OutputTokens + c.CacheReadTokens + c.CacheWriteTokens
}

// Budget is a run's envelope (PRD FO-03). Zero on any field means "no ceiling
// for this dimension".
type Budget struct {
	Micros      int64
	Tokens      int64
	ToolCalls   int64
	Steps       int64
	WallClockMS int64
}

// Consumption is what a run has already spent or reserved.
type Consumption struct {
	Micros      int64 `json:"micros,omitempty"`
	Tokens      int64 `json:"tokens,omitempty"`
	ToolCalls   int64 `json:"tool_calls,omitempty"`
	Steps       int64 `json:"steps,omitempty"`
	WallClockMS int64 `json:"wall_clock_ms,omitempty"`
}

// Exceeds reports which ceiling was breached, or an empty string when the
// consumption fits.
//
// The check runs in the Gate against the *reserved* amount, never against a
// total accumulated after the call: between spending and accounting there is
// a window in which parallel steps blow through the ceiling (PRD FO-01).
func (b Budget) Exceeds(c Consumption) string {
	switch {
	case b.Micros > 0 && c.Micros > b.Micros:
		return "cost"
	case b.Tokens > 0 && c.Tokens > b.Tokens:
		return "tokens"
	case b.ToolCalls > 0 && c.ToolCalls > b.ToolCalls:
		return "tool calls"
	case b.Steps > 0 && c.Steps > b.Steps:
		return "steps"
	case b.WallClockMS > 0 && c.WallClockMS > b.WallClockMS:
		return "wall clock"
	}
	return ""
}

// Narrow returns the tighter of b and parent, dimension by dimension. It backs
// ceiling inheritance down the scope hierarchy — installation, company, area,
// capability pack, agent — where no level may widen the one above it
// (PRD 3.1, FO-02).
func (b Budget) Narrow(parent Budget) Budget {
	return Budget{
		Micros:      minPositive(b.Micros, parent.Micros),
		Tokens:      minPositive(b.Tokens, parent.Tokens),
		ToolCalls:   minPositive(b.ToolCalls, parent.ToolCalls),
		Steps:       minPositive(b.Steps, parent.Steps),
		WallClockMS: minPositive(b.WallClockMS, parent.WallClockMS),
	}
}

// minPositive treats zero as "no ceiling" rather than as the smallest value.
func minPositive(a, b int64) int64 {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	}
	return b
}
