package httpapi

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// ProviderNames answers whether a model provider is one this installation
// configures, from the administration area or the environment. Declared here
// by the consumer.
type ProviderNames interface {
	Configured(name string) bool
}

// WithProviderNames wires where a fallback provider is checked at publish.
func (s *Server) WithProviderNames(p ProviderNames) *Server {
	s.providerNames = p
	return s
}

/*
unconfiguredFallback is the refusal for a fallback nobody can ask.

Checked at publish rather than found out at run time, because the moment it
would otherwise be found out is the outage the fallback exists for. Unwired —
a laptop, a test — there is no list to check against, and the parser has
already refused what the definition says about itself.
*/
func (s *Server) unconfiguredFallback(published spec.Spec) string {
	if published.Fallback.IsZero() || s.providerNames == nil {
		return ""
	}
	if s.providerNames.Configured(published.Fallback.Provider) {
		return ""
	}
	return fmt.Sprintf("the fallback provider %q is not one this installation configures",
		published.Fallback.Provider)
}

func pairOf(in *openapi.ModelPair) domain.ModelPair {
	if in == nil {
		return domain.ModelPair{}
	}
	return domain.ModelPair{Provider: in.Provider, Model: in.Model}
}

func pairFrom(p domain.ModelPair) *openapi.ModelPair {
	if p.IsZero() {
		return nil
	}
	return &openapi.ModelPair{Provider: p.Provider, Model: p.Model}
}

// runFallback marks a run that switched to its fallback pair, so the evidence
// it produced can be weighed apart from runs on the pair the agent was
// promoted on.
func runFallback(steps []domain.Step) *openapi.RunFallback {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Kind != domain.StepModelFallback {
			continue
		}
		var p domain.ModelFallbackPayload
		if err := json.Unmarshal(steps[i].Payload, &p); err != nil {
			return nil
		}
		return &openapi.RunFallback{
			From: openapi.ModelPair{Provider: p.From.Provider, Model: p.From.Model},
			To:   openapi.ModelPair{Provider: p.To.Provider, Model: p.To.Model},
			Code: p.Failure.Code, AtSeq: steps[i].Seq,
		}
	}
	return nil
}
