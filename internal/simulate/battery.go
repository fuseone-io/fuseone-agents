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

	rows := make(map[string]int, len(report.Cases))
	for i, c := range report.Cases {
		if c.ID != "" {
			rows[c.ID] = i
		}
	}

	out := report
	out.Cases = append([]Case(nil), report.Cases...)
	out.Held, out.Broken = 0, 0

	for _, corrected := range corpus {
		at, ran := rows[corrected.ID]
		if !ran {
			out.Cases = append(out.Cases, Case{
				ID: corrected.ID, Settled: SettledUnsettled,
				Expected: corrected.Expectations, Unmet: corrected.Expectations,
				Error: "the case did not run",
			})
			out.Broken++
			continue
		}

		out.Cases[at].Expected = corrected.Expectations
		out.Cases[at].Unmet = Check(out.Cases[at], corrected.Expectations)
		if len(out.Cases[at].Unmet) == 0 {
			out.Held++
		} else {
			out.Broken++
		}
	}
	return out
}
