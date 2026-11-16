package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

/*
An instruction is counted by the provider a run of the agent would reach
(NT-019, phase 3 writers): the request names the agent's scope, and the counter
is asked at it. Without a scope only the installation's providers count, as
before.
*/

type countsAt struct{ asked *[]domain.Scope }

func (c countsAt) Counter(_ string, at domain.Scope, _ model.Config) (model.Counter, error) {
	*c.asked = append(*c.asked, at)
	return countsTo(12), nil
}

func countAt(scope *string) openapi.CountInstructionTokensRequestObject {
	return openapi.CountInstructionTokensRequestObject{Body: &openapi.CountInstructionTokensJSONRequestBody{
		Provider: "cx-claude", Model: "claude-opus-5", Instructions: "Answer only about refunds.", Scope: scope,
	}}
}

func TestCountInstructionTokens_isAskedAtTheAgentsScope(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		scope *string
		want  domain.Scope
	}{
		{ptr("acme/cx"), domain.Scope{Company: "acme", Area: "cx"}},
		{nil, domain.Scope{Company: domain.Installation}},
	} {
		var asked []domain.Scope
		resp, err := NewServer(nil, "test").WithTokenisers(countsAt{&asked}).
			CountInstructionTokens(inArea("cx", domain.RoleAuthor), countAt(c.scope))
		if err != nil {
			t.Fatalf("CountInstructionTokens: %v", err)
		}
		if _, ok := resp.(openapi.CountInstructionTokens200JSONResponse); !ok || len(asked) != 1 || asked[0] != c.want {
			t.Errorf("scope %v: response %T, asked at %v; want %v", c.scope, resp, asked, c.want)
		}
	}
}

// The count reaches a provider with that scope's key: the caller must be one
// who may publish there.
func TestCountInstructionTokens_atAScopeTheCallerCannotPublishIn_isRefused(t *testing.T) {
	t.Parallel()
	var asked []domain.Scope
	resp, err := NewServer(nil, "test").WithTokenisers(countsAt{&asked}).
		CountInstructionTokens(inArea("ops", domain.RoleAuthor), countAt(ptr("acme/cx")))
	if err != nil {
		t.Fatalf("CountInstructionTokens: %v", err)
	}
	if _, refused := resp.(openapi.CountInstructionTokens403ApplicationProblemPlusJSONResponse); !refused || len(asked) != 0 {
		t.Fatalf("response = %T, asked %v; want 403 and no provider asked", resp, asked)
	}
}
