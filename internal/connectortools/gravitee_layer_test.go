package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

func TestLayer_routesTheRealGraviteeToolWithoutTheMCPResultCache(t *testing.T) {
	fixture := newAcceptanceFixture(t)
	base := &cachingBase{}
	gravitee := graviteeInstance(fixture.call.Scope, graviteeSource("secrets"))
	vault := graviteeVault("secrets", fixture.call.Scope)
	layer := New(base, nil, fixture.content, nil).WithGraviteeRuntime(fixture.runtime)
	if err := layer.SetInstances([]Instance{gravitee, vault}); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	args, _ := json.Marshal(fixture.input)
	fixture.call.Tool = "gravitee.apim.accept_subscription"
	fixture.call.Args = args
	fixture.call.ContractDigest = layer.ApprovalBinding(fixture.call)
	fixture.access.access.ContractDigest = fixture.call.ContractDigest
	if err := layer.Reserve(t.Context(), fixture.call); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	result, err := layer.Invoke(t.Context(), fixture.call)
	if err != nil || result.Failed || base.invoked != 0 || fixture.remote.acceptCalls != 1 {
		t.Fatalf("Invoke = (%+v, %v), base=%d POST=%d",
			result, err, base.invoked, fixture.remote.acceptCalls)
	}
}

func TestLayer_refusesExtraGraviteeArgumentsBeforeResolvingAuthority(t *testing.T) {
	fixture := newAcceptanceFixture(t)
	layer := New(nil, nil, fixture.content, nil).WithGraviteeRuntime(fixture.runtime)
	if err := layer.SetInstances([]Instance{
		graviteeInstance(fixture.call.Scope, graviteeSource("secrets")),
		graviteeVault("secrets", fixture.call.Scope),
	}); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	fixture.call.Tool = "gravitee.apim.accept_subscription"
	fixture.call.Args = []byte(`{
		"subscriptionId":"sub-42",
		"expiresAt":"2026-09-13T15:00:00Z",
		"customApiKey":"must-never-be-accepted"
	}`)
	result, err := layer.Invoke(t.Context(), fixture.call)
	if err != nil || !result.Failed || result.ErrorCode != CodeConnectorBadArguments ||
		fixture.access.calls != 0 || fixture.remote.acceptCalls != 0 {
		t.Fatalf("Invoke = (%+v, %v), authority=%d POST=%d",
			result, err, fixture.access.calls, fixture.remote.acceptCalls)
	}
}

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
