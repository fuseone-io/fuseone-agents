package connectortools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/ticket"
)

func TestGraviteeTicketOutcome_saysExpiryAndSecurityWithoutCarryingAKey(t *testing.T) {
	t.Parallel()
	expires := "2026-09-30T18:00:00Z"
	raw, err := json.Marshal(GraviteeAcceptanceResult{
		Operation: "gravitee.accept_subscription", Status: "accepted",
		SubscriptionID: "sub-42", Application: GraviteeResource{ID: "app-1", Name: "Payments"},
		API:                 GraviteeResource{ID: "api-1", Name: "Checkout"},
		Plan:                GraviteeResource{ID: "plan-1", Name: "API Key"},
		RequestedExpiration: &expires, DecidedBy: "usr_manager",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := (GraviteeTicketOutcomeRenderer{}).RenderTicketOutcome(ticket.PhaseCompleted, raw)
	if err != nil {
		t.Fatalf("RenderTicketOutcome: %v", err)
	}
	for _, want := range []string{"approved in Gravitee", expires, "never posts the API key", "secret manager", "revoke or rotate"} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q; missing %q", message, want)
		}
	}
	for _, forbidden := range []string{"Payments", "Checkout", "usr_manager", "api-key-canary"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("message exposed %q", forbidden)
		}
	}
}

func TestGraviteeTicketOutcome_refusesAResultWithAnyExtraSecretShapedField(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"operation":"gravitee.accept_subscription","status":"accepted",
		"subscriptionId":"sub-42","application":{"id":"app-1","name":"Payments"},
		"api":{"id":"api-1","name":"Checkout"},"plan":{"id":"plan-1","name":"Key"},
		"decidedBy":"usr_manager","apiKey":"api-key-canary"}`)
	if message, err := (GraviteeTicketOutcomeRenderer{}).RenderTicketOutcome(ticket.PhaseCompleted, raw); err == nil || message != "" {
		t.Fatalf("RenderTicketOutcome = (%q, %v), want a closed failure", message, err)
	}
}

func TestGraviteeTicketOutcome_manualStateWarnsAgainstASecondWrite(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(GraviteeAcceptanceResult{
		Operation: "gravitee.accept_subscription", Status: CodeConnectorNeedsAttention,
		SubscriptionID: "sub-42", DecidedBy: "usr_manager",
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := (GraviteeTicketOutcomeRenderer{}).
		RenderTicketOutcome(ticket.PhaseNeedsAttention, raw)
	if err != nil {
		t.Fatalf("RenderTicketOutcome: %v", err)
	}
	for _, want := range []string{"could not prove", "Do not retry", "inspect", "keep observing"} {
		if !strings.Contains(message, want) {
			t.Errorf("message = %q; missing %q", message, want)
		}
	}
}
