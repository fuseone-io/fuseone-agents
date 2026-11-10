package simulate

import (
	"slices"
	"sort"

	"github.com/fuseone/agents/internal/domain"
)

/*
Two versions on one corpus.

The question somebody is actually asking when they publish is not "does v4
pass" but "is v4 better than v3", and two reports side by side do not answer
it: the reader is left matching case identifiers by eye, which works for five
cases and fails silently at forty.

Everything this needs is already stored. A battery is a set of simulated runs
naming a version, and a report is a fold of them — so a comparison is a fold
of two folds, with no third thing to keep in step.
*/

// Standing is how one case stood in one battery.
type Standing string

const (
	Held  Standing = "held"
	Broke Standing = "broke"
	// Absent is a case that battery never ran. Reported rather than dropped:
	// a corpus that grew between the two versions is a real difference, and
	// answering "nothing changed" about it is the one wrong answer.
	Absent Standing = "absent"
	// NotComparable is a case the two batteries did not ask the same question about:
	// only one version of the corpus had it, or it expected something else in each. It
	// counts as neither a regression nor a fix, because nobody measured either.
	NotComparable Standing = "not_comparable"
)

// CaseChange is what happened to one case between the two.
type CaseChange struct {
	ID  string   `json:"id"`
	Was Standing `json:"was"`
	Now Standing `json:"now"`
	// CostMicros and Steps are the movement, new minus old. An agent that
	// reaches the same answer for three times the money is worse, and a
	// comparison of held-and-broken alone reports it as identical.
	CostMicros int64 `json:"cost_micros"`
	Steps      int   `json:"steps"`
}

// Regressed reports the one change worth stopping for.
func (c CaseChange) Regressed() bool { return c.Was == Held && c.Now == Broke }

// Fixed reports a correction that started holding again.
func (c CaseChange) Fixed() bool { return c.Was == Broke && c.Now == Held }

// Comparison is the whole answer.
type Comparison struct {
	From      domain.VersionID `json:"from"`
	To        domain.VersionID `json:"to"`
	Cases     []CaseChange     `json:"cases"`
	Regressed int              `json:"regressed"`
	Fixed     int              `json:"fixed"`
	// CostMicros is the movement over every case both sides ran. Cases only
	// one side ran are left out of it: a total that mixed a rise in price
	// with a case that did not exist yet is a number nobody can act on.
	CostMicros int64 `json:"cost_micros"`
}

// Compare answers what changed between two batteries of the same corpus.
func Compare(was, now Report) Comparison {
	out := Comparison{From: was.Version, To: now.Version}

	before := standings(was)
	after := standings(now)
	// With one version of the corpus on both sides, a case one side lacks is one it did
	// not run. Across two, it may be one that side was never asked.
	missing := Absent
	if was.DatasetVersion != now.DatasetVersion {
		missing = NotComparable
	}

	for id, stood := range before {
		stands, ran := after[id]
		if !ran {
			out.Cases = append(out.Cases, CaseChange{ID: id, Was: stood.standing, Now: missing})
			continue
		}
		change := changeOf(id, stood, stands)
		out.CostMicros += change.CostMicros
		out.Cases = append(out.Cases, change)
	}
	for id, stands := range after {
		if _, ran := before[id]; !ran {
			out.Cases = append(out.Cases, CaseChange{ID: id, Was: missing, Now: stands.standing})
		}
	}
	out.Regressed, out.Fixed = tally(out.Cases)
	// Total, and never only by weight: the rows are built from maps, so ties
	// left unbroken would come back in a different order on every read. This
	// is read on a screen an approver was shown and in an export somebody
	// keeps, and a list that reshuffles between two readings of the same two
	// versions is one nobody can cite.
	sort.Slice(out.Cases, func(i, j int) bool {
		a, b := weigh(out.Cases[i]), weigh(out.Cases[j])
		if a != b {
			return a < b
		}
		return out.Cases[i].ID < out.Cases[j].ID
	})
	return out
}

func tally(cases []CaseChange) (regressed, fixed int) {
	for _, c := range cases {
		switch {
		case c.Regressed():
			regressed++
		case c.Fixed():
			fixed++
		}
	}
	return regressed, fixed
}

// changeOf is one case both sides have. Expecting something else in each, it was not
// asked the same question twice, and its movement says nothing about the agent. One side
// that never ran it stands broken there, and moves no cost or steps: its row costs nothing
// because nothing ran, which is not a saving.
func changeOf(id string, was, now standing) CaseChange {
	if !slices.Equal(was.expected, now.expected) {
		return CaseChange{ID: id, Was: was.standing, Now: NotComparable}
	}
	if !was.ran || !now.ran {
		return CaseChange{ID: id, Was: was.standing, Now: now.standing}
	}
	return CaseChange{
		ID: id, Was: was.standing, Now: now.standing,
		CostMicros: int64(now.cost.Micros) - int64(was.cost.Micros),
		Steps:      now.steps - was.steps,
	}
}

type standing struct {
	standing Standing
	// ran is whether a run of this case opened at all: a battery's row for a case that did
	// not run is a stand-in.
	ran      bool
	cost     domain.Cost
	steps    int
	expected []domain.Expectation
}

// standings indexes a report by case id.
//
// By identifier and never by position: reports are folded in the order the
// ledger holds the runs, which is not the order a corpus was written in.
func standings(r Report) map[string]standing {
	out := make(map[string]standing, len(r.Cases))
	for _, c := range r.Cases {
		// An erased case did not run. Reading it as having held would report the
		// other side as a regression or a fix nobody measured.
		if c.ID == "" || c.Settled == SettledErased {
			continue
		}
		stood := Held
		if len(c.Unmet) > 0 {
			stood = Broke
		}
		// Two runs of one case, read with no corpus to reduce them. Either one breaking
		// means it did not hold.
		if seen, ran := out[c.ID]; ran && seen.standing == Broke {
			continue
		}
		out[c.ID] = standing{
			standing: stood, ran: !c.didNotRun, cost: c.Cost, steps: c.Steps, expected: c.Expected,
		}
	}
	return out
}

// weigh orders the rows so the answer is the first thing read. A comparison
// that lists forty unchanged cases before the one that broke has hidden it.
func weigh(c CaseChange) int {
	switch {
	case c.Regressed():
		return 0
	case c.Now == Broke:
		return 1
	case c.Fixed():
		return 2
	case c.Was == Absent || c.Now == Absent || c.Was == NotComparable || c.Now == NotComparable:
		return 3
	default:
		return 4
	}
}
