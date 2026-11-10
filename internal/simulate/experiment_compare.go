package simulate

import "github.com/fuseone/agents/internal/domain"

/*
ExperimentComparison is two experiments of one agent, case by case (NT-015 § Experiments).

On one dataset version both asked the same questions, and every case is compared. Across
two they did not, and a case one version lacks is not a regression or a fix, only a
question one side was never asked: the comparison lists the cases both had, says
so with SharedOnly, and counts what each side had that the other did not.
*/
type ExperimentComparison struct {
	From, To ExperimentSummary
	// SharedOnly says the two ran different dataset versions, so only shared cases are
	// listed. OnlyFrom and OnlyTo are the cases left out of each side.
	SharedOnly       bool
	OnlyFrom, OnlyTo int

	Cases     []ExperimentCaseChange
	Regressed int
	Fixed     int
	// CostMicros is the movement per run, over the cases both ran, new minus old.
	CostMicros int64
}

// ExperimentCaseChange is one case's change, with in how many repetitions it broke on
// each side.
type ExperimentCaseChange struct {
	CaseChange
	WasBrokenIn, WasRan int
	NowBrokenIn, NowRan int
}

// CompareExperiments answers what changed from one experiment to another.
func CompareExperiments(was, now ExperimentReport) ExperimentComparison {
	compared := Compare(perRun(was), perRun(now))
	out := ExperimentComparison{From: was.ExperimentSummary, To: now.ExperimentSummary}
	before, after := askedCases(was), askedCases(now)
	out.SharedOnly = was.DatasetVersion != now.DatasetVersion
	for _, change := range compared.Cases {
		w, inWas := before[change.ID]
		n, inNow := after[change.ID]
		if out.SharedOnly && !(inWas && inNow) {
			continue
		}
		out.Cases = append(out.Cases, ExperimentCaseChange{
			CaseChange: change, WasBrokenIn: w.BrokenIn, WasRan: w.Ran, NowBrokenIn: n.BrokenIn, NowRan: n.Ran,
		})
	}
	if out.SharedOnly {
		out.OnlyFrom, out.OnlyTo = onlyIn(before, after), onlyIn(after, before)
	}
	// The rows left out are cases one side was never asked, which Compare already reads as
	// not comparable: neither a regression nor a fix, and no cost moved.
	out.Regressed, out.Fixed, out.CostMicros = compared.Regressed, compared.Fixed, compared.CostMicros
	return out
}

// askedCases indexes the cases an experiment's dataset version asked, by id: every case
// of the version that was not erased, including one that did not run, which a battery
// counts as broken and so must a comparison.
func askedCases(r ExperimentReport) map[string]ExperimentCase {
	out := make(map[string]ExperimentCase, len(r.Cases))
	for _, c := range r.Cases {
		if c.ID != "" && c.Settled != SettledErased {
			out[c.ID] = c
		}
	}
	return out
}

func onlyIn(side, other map[string]ExperimentCase) int {
	n := 0
	for id := range side {
		if _, shared := other[id]; !shared {
			n++
		}
	}
	return n
}

// perRun is an experiment as a battery with one run per case, costing what one of its
// repetitions cost on average, so a comparison of five repetitions with one moves by
// price and not by count. Standing is unchanged: the row is still the worst run.
func perRun(r ExperimentReport) Report {
	out := Report{DatasetVersion: r.DatasetVersion, Version: r.Version, Cases: make([]Case, 0, len(r.Cases))}
	for _, c := range r.Cases {
		row := c.Case
		if c.Ran > 0 {
			runs := append([]Case{row}, row.others...)
			steps := 0
			for _, run := range runs {
				steps += run.Steps
			}
			row.Cost = domain.Cost{Micros: row.Spent().Micros / int64(len(runs))}
			row.Steps = steps / len(runs)
		}
		row.others = nil
		out.Cases = append(out.Cases, row)
	}
	return out
}
