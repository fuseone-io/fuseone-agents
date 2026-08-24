package tools

import (
	"errors"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpmetrics"
)

const (
	CodeMCPMetricOther               = mcpmetrics.CodeOther
	CodeMCPNoCode                    = mcpmetrics.CodeNoCode
	CodeMCPInvokeError               = mcpmetrics.CodeInvokeError
	CodeMCPToolError                 = mcpmetrics.CodeToolError
	CodeMCPCacheHit                  = mcpmetrics.CodeCacheHit
	CodeMCPUnknownTool               = mcpmetrics.CodeUnknownTool
	CodeMCPUnknownServer             = mcpmetrics.CodeUnknownServer
	CodeMCPPersonalCredentialMissing = mcpmetrics.CodePersonalCredentialMissing
	CodeMCPPersonalCredentialRead    = mcpmetrics.CodePersonalCredentialRead
	CodeMCPPersonalCredentialInvalid = mcpmetrics.CodePersonalCredentialInvalid
	CodeMCPPersonalCredentialCaller  = mcpmetrics.CodePersonalCredentialCaller
)

// MCPMetricCode bounds a failure code before it can become a metric label.
func MCPMetricCode(code string) string {
	return mcpmetrics.Code(code)
}

// MCPMetricCodes returns the stable vocabulary used by metrics and UI.
func MCPMetricCodes() []string {
	return mcpmetrics.Codes()
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
