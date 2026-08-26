package main

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/policy"
)

func TestRefusalAlertMessage_isScopeWideRatherThanFirstAgent(t *testing.T) {
	t.Parallel()

	message := refusalAlertMessage(policy.RefusalForm{
		Scope:        domain.Scope{Company: "acme", Area: "ops"},
		PolicyCode:   "POL-100",
		Tool:         "crm.delete_account",
		Effect:       domain.EffectDestructive,
		FirstRunID:   "run-ticketito-1",
		FirstAgentID: "ticketito",
	}, "https://agents.example.com")

	if message.Agent != "" {
		t.Fatalf("agent = %q, want a scope-wide refusal alert", message.Agent)
	}
	if message.RunID != "run-ticketito-1" || message.Tool != "crm.delete_account" ||
		message.Link != "https://agents.example.com/runs/run-ticketito-1" {
		t.Fatalf("message = %+v, want the concrete first example without agent routing", message)
	}
}
