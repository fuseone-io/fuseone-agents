package engine

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpmetrics"
)

/*
AwaitingPersonError is a turn refused before the model was called: the run's
envelope names tools from a server only a person can discover, and nobody has
connected an account to discover it as (#483, delivery 3b).

Unlike ToolServerUnavailableError it is not retryable, and the worker ends the
run on it: no backing off brings a person, and a run parked waiting for one
would be resumed onto the same refusal. Connecting an account afterwards does
not resume it; a new run does the work. Recognised by its type, never by its
code: a tool call that finds the caller without a credential carries the same
code and is supervised as it always was.
*/
type AwaitingPersonError struct {
	Server string
	Tools  []domain.ToolID
}

func (e *AwaitingPersonError) Error() string {
	return fmt.Sprintf(
		"engine: tool server %s waits for a person to connect an account; %d envelope tools cannot be offered",
		e.Server, len(e.Tools))
}

func (e *AwaitingPersonError) Summary() domain.FailureSummary {
	return domain.FailureSummary{
		Code: mcpmetrics.CodePersonalCredentialMissing, Provider: e.Server, Retryable: false,
	}
}
