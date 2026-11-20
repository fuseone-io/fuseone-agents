package trigger

import "github.com/fuseone/agents/internal/domain"

// Request is one intention to run an agent.
type Request struct {
	Agent domain.AgentID
	// IdemKey names the intention, not the attempt. Two requests carrying the
	// same key are the same intention however far apart they arrive.
	IdemKey string
	// Trigger is what caused it: manual, cron, webhook, event. It is recorded
	// because "why did this run" is the first question asked about any run
	// nobody remembers starting.
	Trigger string
	// By is who asked, where a person did.
	By domain.UserID
	// Input is what the run is about. Stored outside the ledger.
	Input []byte
	/*
		Labels are what is true about where the input came from and which
		scope the run is allowed to carry.

		Sealed on `run_started`, which the fold unions into the run, so a run
		opened from outside meets the taint check on its very first proposal
		rather than only after an untrusted tool has answered. Without it an
		agent that read a webhook body and wrote straight from it passed the
		check that exists for exactly that.

		A fact about provenance and never a posture: a run the clock opened
		carries no untrusted source label, but it still carries the company and
		area labels of the published agent. Those labels are the data barrier
		future artifacts inherit.
	*/
	Labels domain.Labels
	// Origin is the conversation this ask arrived in, when one did. Sealed on
	// the opening step, which is what bounds where a reply may go: the run
	// answers where it was asked, and nowhere else is a decision the platform
	// would be making.
	Origin *domain.RunOrigin
	// Ticket is trusted context assembled by the channel ticket admission
	// path. It is never parsed from model-authored input.
	Ticket *domain.TicketContext
	// ContextArtifacts are references an event grants to this run. The opener
	// checks their labels against the target scope before storing anything, so
	// a miswired listener cannot make the model see cross-scope content.
	ContextArtifacts []domain.ContextArtifact
	// Simulation names the batch this run belongs to, and opening a run under
	// one is what marks it simulated: never claimed by a worker, never
	// counted as production. One field rather than a name beside a flag,
	// because the failure a flag allows is a run marked as a simulation that
	// a worker claims and executes for real.
	Simulation string
	// Case names the regression case being replayed, when one is.
	Case string
	// DatasetVersion is the version of the corpus that case was read from.
	DatasetVersion int64
	// Experiment names the experiment a simulated run belongs to, and ExperimentConfig is
	// what it overrides. Only an experiment carries a configuration or a repetition count
	// (experiment.go).
	Experiment            string
	ExperimentConfig      *domain.ExperimentConfig
	ExperimentRepetitions int
	// Version pins the run to one published version of the agent. Empty is
	// the version published now, which is what every trigger but one wants.
	Version domain.VersionID
	// SealID names the candidate an experiment's or a rehearsal's simulated run executes
	// (NT-018 phases 2 and 4). Any other request naming one is refused as a version the
	// agent never published.
	SealID string
	// Rehearsal marks a live rehearsal (NT-018 phase 4): a simulated run of a candidate
	// whose calls wait for its author. Honoured only with a Simulation, a SealID and a
	// By, and never with an Experiment; anywhere else it is refused, never dropped. Its
	// conversation is resolved by the opener (open_rehearsal.go), never named here.
	Rehearsal bool
	// SourceRun is the recorded run whose input seeds this one, when an
	// operator asked to run from it (#16). An id and nothing else: the trail
	// points at the source, it does not copy it.
	SourceRun domain.RunID
	// Chain is where a run an event opens stands in the chain of runs events
	// opened (#203). Set by the dispatcher alone, from the source run's own
	// recorded start, and sealed so the next hop reads it from the ledger.
	Chain *domain.EventChain
	// WorkItem is the card the run is attributed to by how it was opened
	// (#204, slice 3), attribution only. Trusted context like Chain and
	// Ticket: set by the dispatcher from the source run's record and by the
	// GitHub door from a signed delivery's structural fields, never parsed
	// from Input or anything else a model or a sender wrote.
	WorkItem *domain.RunWorkItem
}
