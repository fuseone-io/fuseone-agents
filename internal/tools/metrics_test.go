package tools_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/tools"
)

func TestMCPMetricCode_boundsUnknownCodes(t *testing.T) {
	if got := tools.MCPMetricCode(tools.CodeMCPPersonalCredentialMissing); got != tools.CodeMCPPersonalCredentialMissing {
		t.Fatalf("known code = %q", got)
	}
	if got := tools.MCPMetricCode("jira-prod.transition_ACME-4417"); got != tools.CodeMCPMetricOther {
		t.Fatalf("unknown code = %q, want other", got)
	}
}

func TestMCPMetricCodes_exposesTheBoundedVocabulary(t *testing.T) {
	codes := tools.MCPMetricCodes()
	for _, want := range []string{
		tools.CodeMCPMetricOther,
		tools.CodeMCPPersonalCredentialMissing,
		tools.CodeMCPServerRateLimited,
	} {
		if !slices.Contains(codes, want) {
			t.Fatalf("codes %v missing %q", codes, want)
		}
	}
}
