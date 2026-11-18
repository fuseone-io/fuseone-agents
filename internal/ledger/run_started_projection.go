package ledger

import (
	"encoding/json"
	"regexp"

	"github.com/fuseone/agents/internal/domain"
)

// startedFacts is what the runs projection learns from a run's opening step.
// Read from the step rather than passed beside it: the ledger is the record,
// and a projection that learned this from somewhere else could disagree with
// the thing it projects.
type startedFacts struct {
	simulated      bool
	simulation     string
	ticketKey      string
	ticketRevision int64
	// seal and thread are nil unless the step names a well-formed id. A
	// malformed one must not stop the step from being recorded: the worker
	// refuses the run, and the projection simply has nothing to follow.
	seal, thread *string
}

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func startedFactsOf(s domain.Step) startedFacts {
	var out startedFacts
	if s.Kind != domain.StepRunStarted {
		return out
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(s.Payload, &started); err != nil {
		return out
	}
	out.simulated, out.simulation = started.Simulated, started.Simulation
	if started.Ticket != nil && started.Ticket.Ref.Valid() {
		out.ticketKey = string(started.Ticket.Ref.Key)
		out.ticketRevision = started.Ticket.Ref.Revision
	}
	out.seal = uuidOrNil(started.Seal)
	out.thread = uuidOrNil(started.AuthoringThread)
	return out
}

func uuidOrNil(id string) *string {
	if !uuidShape.MatchString(id) {
		return nil
	}
	return &id
}
