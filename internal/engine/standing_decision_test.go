package engine

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A rolling upgrade: during it, a binary of the previous release folds
approval_decided steps the new one wrote with standing_grant, a field it does
not know. The fold decodes decisions without refusing unknown fields, so the
older binary reads the verdict and ignores the rest — exactly as this one
must, when a later release adds a field of its own. The raw payload here is
what the new engine writes, plus a field no release knows yet.
*/
func TestFold_aDecisionCarryingFieldsThisBinaryDoesNotKnow_stillReleasesTheRun(t *testing.T) {
	t.Parallel()
	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{
			Kind:    domain.StepApprovalRequested,
			Payload: payload(t, domain.ApprovalRequestedPayload{Tool: "crm.reply"}),
		},
		domain.Step{
			Kind: domain.StepApprovalDecided,
			Payload: []byte(`{"approved":true,"by":"usr_ana","note":"standing approval SG-7: night",` +
				`"at_seq":2,"standing_grant":"SG-7","a_field_from_a_later_release":{"x":1}}`),
		},
	)

	s := mustFold(t, steps)

	if s.Phase != PhaseRunning || s.PendingApproval != nil {
		t.Fatalf("Phase = %v, pending = %+v; want the decision applied", s.Phase, s.PendingApproval)
	}
}
