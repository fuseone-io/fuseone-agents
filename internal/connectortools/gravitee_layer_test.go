package connectortools

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func TestLayer_onlyTheGraviteeAcceptanceMayClaimInspectedApprovalEvidence(t *testing.T) {
	t.Parallel()
	scope := area("acme", "platform")
	want := domain.ApprovalEvidence{
		Kind:   ApprovalEvidenceGraviteeSubscription,
		Ticket: domain.TicketRef{Key: "ticket", Revision: 1},
		Ref:    "content://snapshot", Digest: "sha256:snapshot",
	}
	provider := &approvalEvidenceSpy{evidence: want}
	layer := New(nil, nil, nil, nil).WithGraviteeInspector(provider)
	if err := layer.SetInstances([]Instance{graviteeInstance(scope, graviteeSource("secrets"))}); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}

	got, err := layer.ApprovalEvidence(t.Context(), engine.Call{
		Tool: "gravitee.apim.accept_subscription", Scope: scope,
	})
	if err != nil || got != want || provider.calls != 1 {
		t.Fatalf("accept evidence = (%+v, %v), calls = %d", got, err, provider.calls)
	}
	got, err = layer.ApprovalEvidence(t.Context(), engine.Call{
		Tool: "gravitee.apim.inspect_subscription", Scope: scope,
	})
	if err != nil || !got.Empty() || provider.calls != 1 {
		t.Fatalf("inspect evidence = (%+v, %v), calls = %d", got, err, provider.calls)
	}
}

func TestLayer_bindsGraviteeApprovalToConfigurationAndInspectedEvidence(t *testing.T) {
	t.Parallel()
	scope := area("acme", "platform")
	gravitee := graviteeInstance(scope, graviteeSource("secrets"))
	vault := graviteeVault("secrets", scope)
	layer := New(nil, nil, nil, nil)
	if err := layer.SetInstances([]Instance{gravitee, vault}); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	call := engine.Call{
		Tool: "gravitee.apim.accept_subscription", Scope: scope,
		Ticket: domain.TicketContext{
			Ref: domain.TicketRef{Key: "ticket", Revision: 1}, RequestedBy: "usr_dev",
		},
	}
	call.ContractDigest = layer.ApprovalBinding(call)
	call.ApprovalEvidence = domain.ApprovalEvidence{
		Kind: ApprovalEvidenceGraviteeSubscription, Ticket: call.Ticket.Ref,
		Ref: "content://snapshot", Digest: "sha256:snapshot",
	}
	if call.ContractDigest == "" {
		t.Fatal("Gravitee approval has no server-owned contract digest")
	}
	if err := layer.Reserve(t.Context(), call); err != nil {
		t.Fatalf("Reserve valid approval: %v", err)
	}

	call.ApprovalEvidence = domain.ApprovalEvidence{}
	if err := layer.Reserve(t.Context(), call); !errors.Is(err, ErrGraviteeEvidence) {
		t.Fatalf("Reserve without evidence = %v, want ErrGraviteeEvidence", err)
	}
	call.ApprovalEvidence = domain.ApprovalEvidence{
		Kind: ApprovalEvidenceGraviteeSubscription, Ticket: call.Ticket.Ref,
		Ref: "content://snapshot", Digest: "sha256:snapshot",
	}
	gravitee.Gravitee.MaxTTLSeconds++
	if err := layer.SetInstances([]Instance{gravitee, vault}); err != nil {
		t.Fatalf("SetInstances changed: %v", err)
	}
	if err := layer.Reserve(t.Context(), call); !errors.Is(err, ErrGraviteeContract) {
		t.Fatalf("Reserve changed contract = %v, want ErrGraviteeContract", err)
	}
}

func TestLayer_ownsACopyOfTheGraviteeReferenceBehindTheApproval(t *testing.T) {
	t.Parallel()
	scope := area("acme", "platform")
	gravitee := graviteeInstance(scope, graviteeSource("secrets"))
	layer := New(nil, nil, nil, nil)
	if err := layer.SetInstances([]Instance{gravitee, graviteeVault("secrets", scope)}); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	call := engine.Call{Tool: "gravitee.apim.accept_subscription", Scope: scope}
	before := layer.ApprovalBinding(call)
	gravitee.Gravitee.AllowedReferences[0].ID = "another-api"
	if after := layer.ApprovalBinding(call); after != before {
		t.Fatalf("contract changed through caller-owned slice: %q != %q", after, before)
	}
}

type approvalEvidenceSpy struct {
	evidence domain.ApprovalEvidence
	calls    int
}

func (s *approvalEvidenceSpy) ApprovalEvidence(
	context.Context, engine.Call,
) (domain.ApprovalEvidence, error) {
	s.calls++
	return s.evidence, nil
}
