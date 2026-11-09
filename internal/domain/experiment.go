package domain

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
