package httpapi

import (
	"context"
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
)

/*
The size of an instruction, answered by the model that will read it.

The console has no tokeniser and cannot acquire one that stays right: it
differs between vendors and between generations of the same vendor. So the
number is the provider's own, and where the provider has none the reply says
so and carries no number — the console then shows characters and calls them
characters.
*/

func TestCountInstructionTokens_answersWhatTheProviderCounted(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithTokenisers(countsTo(412))

	resp, err := s.CountInstructionTokens(everywhere(domain.RoleCurator), countRequest("anthropic"))
	if err != nil {
		t.Fatalf("CountInstructionTokens: %v", err)
	}

	answer, ok := resp.(openapi.CountInstructionTokens200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the count", resp)
	}
	if !answer.Counted || answer.Tokens == nil || *answer.Tokens != 412 {
		t.Errorf("got %+v, want the provider's 412", answer)
	}
	// Named, because a count without the model that produced it is the same
	// wrong number in a different unit.
	if answer.Model != "claude-opus-5" {
		t.Errorf("model = %q, want the one that counted", answer.Model)
	}
}

func TestCountInstructionTokens_aProviderThatCannotCount_carriesNoNumber(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithTokenisers(cannotCount{})

	resp, err := s.CountInstructionTokens(everywhere(domain.RoleCurator), countRequest("vllm"))
	if err != nil {
		t.Fatalf("CountInstructionTokens: %v", err)
	}

	answer, ok := resp.(openapi.CountInstructionTokens200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want an answer rather than a failure", resp)
	}
	if answer.Counted || answer.Tokens != nil {
		t.Errorf("got %+v, want no number at all", answer)
	}
}

// Reading what an instruction costs is part of writing one, so it is held by
// whoever may publish. It reaches a configured provider with the
// installation's credential, and a reader of runs has no business spending it.
func TestCountInstructionTokens_withoutAuthorityToPublish_isRefused(t *testing.T) {
	t.Parallel()
	s := NewServer(ledger.NewMemory(), "test").WithTokenisers(countsTo(412))

	resp, err := s.CountInstructionTokens(inCompany("acme", domain.RoleAuditor), countRequest("anthropic"))
	if err != nil {
		t.Fatalf("CountInstructionTokens: %v", err)
	}
	if _, refused := resp.(openapi.CountInstructionTokens403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
}

// --- stubs ------------------------------------------------------------------

func countRequest(provider string) openapi.CountInstructionTokensRequestObject {
	return openapi.CountInstructionTokensRequestObject{
		Body: &openapi.CountInstructionTokensJSONRequestBody{
			Provider: provider, Model: "claude-opus-5",
			Instructions: "Answer only about refunds.",
		},
	}
}

type countsTo int64

func (c countsTo) Counter(string, model.Config) (model.Counter, error) { return c, nil }

func (c countsTo) Count(context.Context, string) (int64, error) { return int64(c), nil }

type cannotCount struct{}

func (cannotCount) Counter(name string, _ model.Config) (model.Counter, error) {
	return nil, fmt.Errorf("%s: %w", name, model.ErrNoTokeniser)
}
