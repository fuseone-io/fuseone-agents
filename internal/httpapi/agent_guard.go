package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
agentGuard is what a write to an agent asks of the agent, from its headers
(NT-017 phase 1, NT-018 1a).

Read by syntax alone, before anything is stored or judged: a header that does
not parse, or both at once, is a 400 that depends on nothing an unauthorised
caller could learn from. The rest is judged by the store, under the agent's
row lock: who may write where the agent lives now, then the precondition.
*/
func agentGuard(ctx context.Context, ifMatch, ifNoneMatch *string) (spec.Guard, string) {
	if ifMatch != nil && ifNoneMatch != nil {
		return spec.Guard{}, "send If-Match to change an agent or If-None-Match: * to create one, not both"
	}
	expect, err := domain.ParseIfMatch(ifMatch)
	if err != nil {
		return spec.Guard{}, err.Error()
	}
	absent, err := domain.ParseIfNoneMatch(ifNoneMatch)
	if err != nil {
		return spec.Guard{}, err.Error()
	}
	return spec.Guard{Allow: mayPublishAt(ctx), Expect: expect, Absent: absent}, ""
}

// mayPublishAt answers, for the agent's current scope read under its lock,
// whether the caller holds agent:publish there.
func mayPublishAt(ctx context.Context) func(domain.Scope) bool {
	return func(current domain.Scope) bool {
		return auth.Require(ctx, domain.PermAgentPublish, current) == nil
	}
}

// agentAbsent is a write the guard answered as an agent the caller cannot reach.
func agentAbsent(err error) bool { return errors.Is(err, spec.ErrAgentNotFound) }
