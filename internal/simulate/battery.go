package simulate

import "github.com/fuseone/agents/internal/domain"

/*
Battery reads a report against the corpus that produced it.

Every case is matched to its own correction by id rather than by position:
runs are folded in the order the ledger holds them, which is not the order a
corpus was written in, and checking case three against case one's correction
reports a failure nobody can act on while hiding a real one.

A corpus case with no row in the report is reported as broken. The corpus is
what the battery promises to check, and reporting only what ran would make the
promise quietly smaller than it says — which is the failure mode a safety net
must not have, because it keeps showing green.
*/
func Battery(report Report, corpus []domain.RegressionCase) Report {
	if len(corpus) == 0 {
		return report
	}

	out := report
	out.Cases = oneRowPerCase(report.Cases, corpus)
	rows := make(map[string]int, len(out.Cases))
	for i, c := range out.Cases {
		if c.ID != "" {
			rows[c.ID] = i
		}
	}

	out.Held, out.Broken, out.Drifted, out.Erased = 0, 0, 0, 0

	for _, corrected := range corpus {
		at, ran := rows[corrected.ID]
		// A gap, not a failure. The case cannot run again, and calling it broken
		// would accuse the agent of something nobody checked. Only when it did
		// not run: a past battery that did run it keeps saying what happened.
		if !ran && corrected.Erased {
			out.Cases = append(out.Cases, Case{
				ID: corrected.ID, Settled: SettledErased, Expected: corrected.Expectations,
				Error: "the input of this case was erased",
			})
			out.Erased++
			continue
		}
		if !ran {
			out.Cases = append(out.Cases, Case{
				ID: corrected.ID, Settled: SettledUnsettled, didNotRun: true,
				Expected: corrected.Expectations, Unmet: corrected.Expectations,
				Error: "the case did not run",
			})
			out.Broken++
			continue
		}

		// Erasure took what it expected too. There is nothing left to judge the run
		// against, and no unmet expectations would otherwise read as a case that held.
		if corrected.Erased && len(corrected.Expectations) == 0 {
			out.Cases[at].Settled = SettledErased
			out.Cases[at].Error = "the input and the expectations of this case were erased"
			out.Erased++
			continue
		}

		out.Cases[at].Expected = corrected.Expectations
		// Said whether or not the case broke. A case that still holds against
		// a model that moved is worth knowing about too: it is the one that
		// will break next.
		out.Cases[at].Drifted = drifted(out.Cases[at], corrected)
		out.Cases[at].Unmet = Check(out.Cases[at], corrected.Expectations)
		if out.Cases[at].Drifted {
			out.Drifted++
		}
		if len(out.Cases[at].Unmet) == 0 {
			out.Held++
		} else {
			out.Broken++
		}
	}
	return out
}

/*
oneRowPerCase reduces a battery to one row for each case, the worst of its runs.

A case can have two: a pod of the previous release keys a battery's runs by position and a
pod of this one by case, so during a rollout they can open one window twice. The runs can
disagree, and judging either one alone could report as held a case that just broke, and let
an agent start. So a case holds only if every run of it did.

The row is one real run, because an expectation is about one run: two cheap runs are not
one expensive one. The others stay on it, for what the whole battery has to say about them:
what they cost, and whether a Gate stopped them. A case with no name is its own row.
*/
func oneRowPerCase(cases []Case, corpus []domain.RegressionCase) []Case {
	expected := make(map[string][]domain.Expectation, len(corpus))
	for _, c := range corpus {
		expected[c.ID] = c.Expectations
	}
	out := make([]Case, 0, len(cases))
	at := make(map[string]int, len(cases))
	for _, c := range cases {
		first, seen := at[c.ID]
		if c.ID == "" || !seen {
			at[c.ID] = len(out)
			out = append(out, c)
			continue
		}
		out[first] = worseOf(out[first], c, expected[c.ID])
	}
	return out
}

func worseOf(row, run Case, expected []domain.Expectation) Case {
	others := row.others
	row.others = nil
	worse, other := row, run
	a, b := weight(row, expected), weight(run, expected)
	// By the run id when they weigh the same, so the ledger's order decides nothing.
	if b > a || (b == a && run.RunID < row.RunID) {
		worse, other = run, row
	}
	worse.others = append(others, other)
	return worse
}

// weight orders the runs of one case by what a reader must not miss. A block is not in
// here: the row keeps the other runs, so a block counts wherever it happened.
func weight(c Case, expected []domain.Expectation) int {
	switch {
	case len(Check(c, expected)) > 0:
		return 5
	case c.Settled == SettledUnsettled:
		return 4
	case c.Settled == SettledParked:
		return 3
	case c.Settled == SettledWaiting:
		return 2
	default:
		return 0
	}
}

// drifted says whether any run of a case answered on another model than it last held
// against. Any of them, because a provider moving a model under one run is still moving it.
func drifted(row Case, corrected domain.RegressionCase) bool {
	if onAnotherModel(row, corrected) {
		return true
	}
	for _, other := range row.others {
		if onAnotherModel(other, corrected) {
			return true
		}
	}
	return false
}

func onAnotherModel(run Case, corrected domain.RegressionCase) bool {
	return corrected.Model != "" && run.Model != "" && corrected.Model != run.Model
}
