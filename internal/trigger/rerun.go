package trigger

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// RerunKey names an intention to run again from a recorded run.
//
// The source is part of the key, so a key a client already spent opening some
// other run can never answer this request with that unrelated run.
func RerunKey(source domain.RunID, key string) string {
	return fmt.Sprintf("rerun:%s:%s", source, key)
}

// RehearsalKey names an intention to rehearse a candidate of an agent (NT-018 phase 4).
//
// The agent and the caller are part of it: a rehearsal belongs to its author's
// conversation, so another author spending the same key opens a run of their own rather
// than reaching somebody else's.
func RehearsalKey(agent domain.AgentID, caller domain.UserID, key string) string {
	return fmt.Sprintf("rehearsal:%s:%s:%s", agent, caller, key)
}
