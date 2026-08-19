package httpapi

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

func TestAbandonRun_withoutCancelPermission_isRefusedForRunCancel(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server := NewServer(store, "test").WithAgents(triggerable(t))

	opened, err := server.StartRun(inArea("cx", domain.RoleAuthor), startRequest("intent-abandon"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	run := opened.(openapi.StartRun201JSONResponse)

	resp, err := server.AbandonRun(
		inArea("cx", domain.RoleApprover),
		openapi.AbandonRunRequestObject{
			RunId: run.RunId,
			Body: &openapi.AbandonRunJSONRequestBody{
				Reason: "the incident moved elsewhere",
			},
		},
	)
	if err != nil {
		t.Fatalf("AbandonRun: %v", err)
	}
	refused, ok := resp.(openapi.AbandonRun403ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if refused.Detail == nil || !strings.Contains(*refused.Detail, string(domain.PermRunCancel)) {
		t.Fatalf("detail = %v, want the missing run:cancel permission named", refused.Detail)
	}
}
