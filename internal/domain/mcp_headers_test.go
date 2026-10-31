package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// HTTP header names are case-insensitive, so two spellings of one name are one
// header with two values — and which one is sent depends on map order.
func TestCheckHeaderNames_twoSpellingsOfOneName_areRefusedWithoutTheirValues(t *testing.T) {
	t.Parallel()

	err := domain.CheckHeaderNames(map[string]string{
		"X-MCP-Readonly": "true-secret", "x-mcp-readonly": "false-secret",
	})

	if !errors.Is(err, domain.ErrHeaderCollision) {
		t.Fatalf("err = %v, want ErrHeaderCollision", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v quotes a value", err)
	}
}

func TestCheckHeaderNames_distinctNames_pass(t *testing.T) {
	t.Parallel()

	err := domain.CheckHeaderNames(map[string]string{"X-MCP-Readonly": "true", "X-MCP-Toolsets": "issues"})

	if err != nil {
		t.Fatalf("err = %v, want distinct names accepted", err)
	}
}
