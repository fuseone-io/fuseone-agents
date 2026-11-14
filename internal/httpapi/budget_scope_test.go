package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A ceiling's scope is a real company or area, written once (NT-019, as the
connector and MCP doors already require).

Only the whole path was trimmed, so " acme / cx " named a scope distinct from
acme/cx, and "Acme" one distinct from acme: a ceiling set there governs no
agent of the real scope, and nothing reports it. A path in another form is
refused, never folded, so the ceiling is the one the caller named.
*/
func TestParseBudgetScope_acceptsOnlyCanonicalScopes(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]domain.Scope{
		"":             {},
		"installation": {},
		"acme":         {Company: "acme"},
		"acme/cx":      {Company: "acme", Area: "cx"},
		" acme/cx ":    {Company: "acme", Area: "cx"},
	} {
		got, ok := parseBudgetScope(raw)
		if !ok || got != want {
			t.Errorf("parseBudgetScope(%q) = %v, %v; want %v", raw, got, ok, want)
		}
	}
	for _, raw := range []string{
		"acme / cx", " acme /cx", "acme/ cx", "Acme", "acme/CX", "acme/cx/finance",
		"acme//cx", "/cx", "acme/", "*", "*/cx", "ac me", "acme/c x",
	} {
		if got, ok := parseBudgetScope(raw); ok {
			t.Errorf("parseBudgetScope(%q) = %v, want refused", raw, got)
		}
	}
}

// Removal reads the path by its shape, so a ceiling stored in another form
// before scopes were checked is still reachable.
func TestParseBudgetShape_takesALegacyFormForRemoval(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]domain.Scope{
		"Acme":    {Company: "Acme"},
		"acme/CX": {Company: "acme", Area: "CX"},
	} {
		if got, ok := parseBudgetShape(raw); !ok || got != want {
			t.Errorf("parseBudgetShape(%q) = %v, %v; want %v", raw, got, ok, want)
		}
	}
}
