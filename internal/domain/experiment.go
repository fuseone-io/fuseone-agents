package domain

import "time"

/*
ExperimentConfig is what an experiment runs an agent version under instead of its own
declaration (NT-015 § Experiments). Every field is optional, and an absent one means the
version's own value.

It is recorded on the opening step of every experiment run, beside the experiment's id,
so that the fold can state what was tested. It is never what makes a run an experiment:
an empty configuration is still an experiment, and a run that read as a battery because
nobody overrode anything would be evidence about a version that was never asked.
*/
type ExperimentConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"effort,omitempty"`
}

// IsZero is a configuration that overrides nothing: the version as published.
func (c ExperimentConfig) IsZero() bool { return c == ExperimentConfig{} }

/*
ExperimentListing is one experiment as its runs record it, for a list: when it opened, the
opening step of its first run — which carries the whole intention, the version the opener
pinned and the scope it was recorded under — and how many runs and cases it opened.

Read in one pass over the runs, so a page of experiments costs a page of reads and not one
per run of every experiment on it.
*/
type ExperimentListing struct {
	ID       string
	OpenedAt time.Time
	Opening  Step
	Runs     int
	Cases    int
}
