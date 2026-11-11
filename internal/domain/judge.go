package domain

/*
JudgeOpening is what a judge run records about the judgment it belongs to (NT-016).

A judge run is a simulated run that names the experiment whose output it judges, so every
door that keeps experiments out of batteries, the start gate and FU-10 keeps it out too.
Its own simulation id is the judgment's, so the experiment's record never counts it. This
field is what tells it apart from the experiment's own runs, and every reader that groups
an experiment's runs leaves out a run that carries it.

Only ids, references, digests and configuration: the rubric, the salt and everything the
judge writes are content, under the run's retention and erasure. A value a rubric declares
can classify a person, so nothing here says what the judge thought.
*/
type JudgeOpening struct {
	// Run is the experiment run whose output this run judges.
	Run RunID `json:"run"`
	// RubricDigest is the rubric's version: its text and declaration, hashed.
	RubricDigest string `json:"rubric_digest"`
	RubricRef    string `json:"rubric_ref,omitempty"`
	// SaltRef and SaltDigest name the salt every verdict object of this run is stored
	// with. The salt itself is never written into a step.
	SaltRef    string `json:"salt_ref,omitempty"`
	SaltDigest string `json:"salt_digest,omitempty"`
	// OutputRef is this run's copy of the judged output, and OutputDigest the digest the
	// judged run sealed for it.
	OutputRef    string `json:"output_ref,omitempty"`
	OutputDigest string `json:"output_digest,omitempty"`
	// Provider, Model and Effort are the judge's, never the judged version's.
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort,omitempty"`
	// CeilingMicros is the per-run ceiling the request declared, required and positive.
	CeilingMicros int64 `json:"ceiling_micros"`
	// Repetitions is how many times each output is judged, and Repetition which this is.
	Repetitions int `json:"repetitions"`
	Repetition  int `json:"repetition"`
}
