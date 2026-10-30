package trigger

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// ErrUnknownVersion means the agent exists and never published the version a
// request asked for.
var ErrUnknownVersion = errors.New("trigger: the agent never published that version")

/*
pin chooses the version a run is opened under.

No choice is the version published now, which the registry answers first.
A choice must be one of the agent's own published versions: a version id
is a string a caller typed, and a run pinned to a version nobody published
would be a run no specification explains.
*/
func pin(versions []domain.AgentSummary, asked domain.VersionID) (domain.AgentSummary, error) {
	if asked == "" {
		return versions[0], nil
	}
	for _, version := range versions {
		if version.VersionID == asked {
			return version, nil
		}
	}
	return domain.AgentSummary{}, fmt.Errorf("%w: %s", ErrUnknownVersion, asked)
}
