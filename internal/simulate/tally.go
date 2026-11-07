package simulate

import "github.com/fuseone/agents/internal/domain"

// Tally is the line an author reads before any of the rows.
type Tally struct {
	Cases     int `json:"cases"`
	Finished  int `json:"finished"`
	Parked    int `json:"parked"`
	Waiting   int `json:"waiting"`
	Unsettled int `json:"unsettled"`
	// Stopped counts cases the Gate refused at least once, wherever they
	// ended. Counted apart from where a case settled because a run that was
	// refused and carried on is still one somebody has to look at.
	Stopped int `json:"stopped"`
	// NotRun counts cases that never opened a run at all.
	NotRun int         `json:"not_run"`
	Cost   domain.Cost `json:"cost"`
}

// Tally counts the report. Derived rather than accumulated, for the same
// reason the rows are: one source, read twice, cannot disagree with itself.
func (r Report) Tally() Tally {
	t := Tally{Cases: len(r.Cases)}
	for _, c := range r.Cases {
		t.Cost = t.Cost.Add(c.Spent())
		if c.RunID == "" {
			// A case that could not run because its input is gone is the report's
			// Erased, and counting it here too would read as two missing cases.
			if c.Settled != SettledErased {
				t.NotRun++
			}
			continue
		}
		if stoppedAnywhere(c) {
			t.Stopped++
		}
		switch c.Settled {
		case SettledFinished:
			t.Finished++
		case SettledParked:
			t.Parked++
		case SettledWaiting:
			t.Waiting++
		// It ran and ended, and what judged it has since been erased. The report
		// counts it as a gap, and nothing about it is still going.
		case SettledErased:
		default:
			t.Unsettled++
		}
	}
	return t
}

// Spent is what every run of this case cost. Cost is the one run it is judged on.
func (c Case) Spent() domain.Cost {
	total := c.Cost
	for _, other := range c.others {
		total = total.Add(other.Cost)
	}
	return total
}

// EveryAct is what every run of this case did, so a block in a run that was not the worst
// is still somewhere a reader can see it.
func (c Case) EveryAct() []Act {
	if len(c.others) == 0 {
		return c.Acted
	}
	out := append([]Act(nil), c.Acted...)
	for _, other := range c.others {
		out = append(out, other.Acted...)
	}
	return out
}

func stoppedAnywhere(c Case) bool {
	if stopped(c) {
		return true
	}
	for _, other := range c.others {
		if stopped(other) {
			return true
		}
	}
	return false
}

func stopped(c Case) bool {
	for _, act := range c.Acted {
		if act.Verdict == domain.VerdictBlock {
			return true
		}
	}
	return false
}
