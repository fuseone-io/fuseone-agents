package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// standingOf reads the form's setting into the definition. Allowed is the
// absence of the line, so the file the console renders is the file an author
// would have written. Any other value passes through untouched, for the
// parser to refuse: read as allowed, a value nobody knows would permit grants
// on an agent somebody meant to close to them.
func standingOf(in *openapi.StandingPolicy) domain.StandingPolicy {
	if in == nil || *in == openapi.StandingPolicyAllowed {
		return domain.StandingAllowed
	}
	return domain.StandingPolicy(*in)
}

// standingFrom hands the setting back to the editor. Absent when allowed,
// like the line in the file.
func standingFrom(p domain.StandingPolicy) *openapi.StandingPolicy {
	if !p.Forbidden() {
		return nil
	}
	return ptr(openapi.StandingPolicyForbidden)
}
