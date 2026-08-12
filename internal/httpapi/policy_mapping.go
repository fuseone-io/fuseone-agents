package httpapi

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// policyFrom shapes a stored rule for the wire.
func policyFrom(p domain.Policy) openapi.Policy {
	effects := make([]openapi.Effect, 0, len(p.Effects))
	for _, e := range p.Effects {
		effects = append(effects, openapi.Effect(e.String()))
	}
	agents := make([]string, 0, len(p.Agents))
	for _, a := range p.Agents {
		agents = append(agents, string(a))
	}
	scopes := make([]openapi.Scope, 0, len(p.Scopes))
	for _, s := range p.Scopes {
		scopes = append(scopes, openapi.Scope{Company: string(s.Company), Area: string(s.Area)})
	}
	conditions := make([]openapi.PolicyCondition, 0, len(p.Conditions))
	for _, c := range p.Conditions {
		conditions = append(conditions, openapi.PolicyCondition{
			Field: c.Field, Op: openapi.PolicyConditionOp(c.Op), Value: c.Value,
		})
	}

	return openapi.Policy{
		Code: p.Code,
		// Generated from the same fields the Gate reads, so the screen cannot
		// describe a rule the engine does not run.
		Sentence:   p.Sentence(),
		Name:       p.Name,
		Owner:      ptr(p.Owner),
		Reason:     ptr(p.Reason),
		Resource:   ptr(p.Resource),
		Effects:    &effects,
		Reach:      ptr(openapi.PolicyReach(p.Reach)),
		Scopes:     &scopes,
		Agents:     &agents,
		Conditions: &conditions,
		Effect:     openapi.PolicyEffect(p.Effect),
		Mode:       openapi.PolicyMode(p.Mode),
		Enabled:    ptr(p.Enabled),
	}
}

// policyInto reads a rule that is about to be stored.
//
// Stricter than draftInto by exactly one field: a stored rule needs a name
// because people have to find it again, and a draft does not because a name
// changes nothing about what the rule does.
func policyInto(code string, in openapi.PolicyInput) (domain.Policy, error) {
	if in.Name == "" {
		return domain.Policy{}, errors.New("uma política precisa de um nome")
	}
	return draftInto(code, in)
}

// draftInto reads a rule that is only being evaluated, refusing one that
// cannot mean anything.
func draftInto(code string, in openapi.PolicyInput) (domain.Policy, error) {
	if code == "" {
		return domain.Policy{}, errors.New("uma política precisa de um código")
	}

	p := domain.Policy{
		Code: code, Name: in.Name,
		Owner: valueOr(in.Owner), Reason: valueOr(in.Reason),
		Resource: valueOr(in.Resource),
		Effect:   domain.PolicyEffect(in.Effect),
		Mode:     domain.PolicyMode(in.Mode),
		Reach:    domain.ReachInstallation,
		Enabled:  true,
	}
	if p.Resource == "" {
		p.Resource = "*"
	}
	if in.Reach != nil {
		p.Reach = domain.PolicyReach(*in.Reach)
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}

	for _, e := range valueOr(in.Effects) {
		effect, err := domain.ParseEffect(string(e))
		if err != nil {
			return domain.Policy{}, fmt.Errorf("efeito desconhecido: %s", e)
		}
		p.Effects = append(p.Effects, effect)
	}
	for _, a := range valueOr(in.Agents) {
		p.Agents = append(p.Agents, domain.AgentID(a))
	}
	for _, s := range valueOr(in.Scopes) {
		p.Scopes = append(p.Scopes, domain.Scope{
			Company: domain.CompanyID(s.Company), Area: domain.AreaID(s.Area),
		})
	}
	for _, c := range valueOr(in.Conditions) {
		p.Conditions = append(p.Conditions, domain.Condition{
			Field: c.Field, Op: string(c.Op), Value: c.Value,
		})
	}

	// A reach naming nothing covers nothing, which reads on the screen as a
	// rule in force. Refused rather than stored as a rule that never fires.
	if p.Reach == domain.ReachAgents && len(p.Agents) == 0 {
		return domain.Policy{}, errors.New("uma política por agente precisa nomear ao menos um")
	}
	if p.Reach == domain.ReachScopes && len(p.Scopes) == 0 {
		return domain.Policy{}, errors.New("uma política por escopo precisa nomear ao menos um")
	}
	return p, nil
}
