package tools

import (
	"errors"
	"sort"

	"github.com/fuseone/agents/internal/domain"
)

const (
	CodeMCPMetricOther               = "other"
	CodeMCPNoCode                    = "none"
	CodeMCPInvokeError               = "invoke_error"
	CodeMCPToolError                 = "tool_error"
	CodeMCPCacheHit                  = "cache_hit"
	CodeMCPUnknownTool               = "unknown_tool"
	CodeMCPUnknownServer             = "unknown_server"
	CodeMCPPersonalCredentialMissing = "mcp_personal_credential_missing"
	CodeMCPPersonalCredentialRead    = "mcp_personal_credential_read_failed"
	CodeMCPPersonalCredentialInvalid = "mcp_personal_credential_invalid"
	CodeMCPPersonalCredentialCaller  = "mcp_personal_credential_no_principal"
)

var mcpMetricCodes = map[string]bool{
	CodeMCPCacheHit:                  true,
	CodeMCPInvokeError:               true,
	CodeMCPNoCode:                    true,
	CodeMCPPersonalCredentialCaller:  true,
	CodeMCPPersonalCredentialInvalid: true,
	CodeMCPPersonalCredentialMissing: true,
	CodeMCPPersonalCredentialRead:    true,
	CodeMCPServerRateLimited:         true,
	CodeMCPToolError:                 true,
	CodeMCPUnknownServer:             true,
	CodeMCPUnknownTool:               true,
}

// MCPMetricCode bounds a failure code before it can become a metric label.
func MCPMetricCode(code string) string {
	if mcpMetricCodes[code] {
		return code
	}
	return CodeMCPMetricOther
}

// MCPMetricCodes returns the stable vocabulary used by metrics and UI.
func MCPMetricCodes() []string {
	codes := make([]string, 0, len(mcpMetricCodes)+1)
	for code := range mcpMetricCodes {
		codes = append(codes, code)
	}
	codes = append(codes, CodeMCPMetricOther)
	sort.Strings(codes)
	return codes
}

// Metrics is implemented by the worker's low-cardinality Prometheus registry.
type Metrics interface {
	MCPToolCall(result, code string, cached bool)
	MCPReservationRefused(code string)
}

type failureSummarizer interface {
	Summary() domain.FailureSummary
}

func failureCodeOf(err error, fallback string) string {
	var summarized failureSummarizer
	if errors.As(err, &summarized) {
		if code := summarized.Summary().Code; code != "" {
			return code
		}
	}
	return fallback
}
