package simulate

import "github.com/fuseone/agents/internal/domain"

/*
ExperimentReport is an experiment read against the dataset version it ran (NT-015
§ Experiments, "Reading an experiment").

The summary's fields are in the order a result is read: what was run — dataset version,
agent version, configuration, cases run out of the cases in the version, erased cases,
repetitions — and only then what was found: held, broken and cost. A count of broken
cases read before the reader knows it was three cases of forty, on another model, is a
number nobody can weigh.
*/
type ExperimentReport struct {
	ExperimentSummary
	Cases []ExperimentCase
}

// ExperimentSummary is what an experiment ran and what it found, in reading order.
type ExperimentSummary struct {
	ID             string
	DatasetVersion int64
	Version        domain.VersionID
	Config         domain.ExperimentConfig
	// CasesRun is how many cases of the version got at least one run, out of
	// CasesInVersion. Erased cases are neither: they are counted apart.
	CasesRun       int
	CasesInVersion int
	Erased         int
	Repetitions    int

	Held, Broken int
	// Cost is what every run cost, every repetition included.
	Cost domain.Cost
	// Running is whether any run has yet to settle.
	Running bool
}

/*
ExperimentCase is one case across its repetitions.

The row is the worst run, as in a battery. BrokenIn is how many repetitions broke and Ran
how many opened, so a reader sees "1 of 5" and not only "broken": a case broken in any
repetition is broken, and how often is what tells a flaky model from a wrong one.
*/
type ExperimentCase struct {
	Case
	BrokenIn int
	Ran      int
}

// ReadExperiment reads an experiment's runs against the corpus of its dataset version.
func ReadExperiment(record Record, report Report, corpus []domain.RegressionCase) ExperimentReport {
	battery := Battery(report, corpus)
	out := ExperimentReport{
		ExperimentSummary: ExperimentSummary{
			ID: report.ID, DatasetVersion: record.DatasetVersion, Version: record.Version,
			Config: record.Config, CasesInVersion: len(corpus), Erased: battery.Erased,
			Repetitions: record.Repetitions, Held: battery.Held, Broken: battery.Broken,
			Running: battery.Running,
		},
		Cases: make([]ExperimentCase, 0, len(battery.Cases)),
	}
	for _, row := range battery.Cases {
		c := repetitionsOf(row)
		if c.Ran > 0 && row.Settled != SettledErased {
			out.CasesRun++
		}
		out.Cost = out.Cost.Add(row.Spent())
		out.Cases = append(out.Cases, c)
	}
	return out
}

// repetitionsOf counts the runs of one row and the ones that broke, each judged alone
// against the row's expectations.
func repetitionsOf(row Case) ExperimentCase {
	out := ExperimentCase{Case: row}
	if row.RunID == "" || row.Settled == SettledErased {
		return out
	}
	for _, run := range append([]Case{row}, row.others...) {
		out.Ran++
		if len(Check(run, row.Expected)) > 0 {
			out.BrokenIn++
		}
	}
	return out
}
