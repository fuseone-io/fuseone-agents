package tools

import (
	"context"
	"errors"
	"log/slog"
	"time"

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

// ToolCallHealth receives the stable outcome of concrete tools/call attempts.
type ToolCallHealth interface {
	RecordToolCall(ctx context.Context, obs domain.IntegrationToolCallObservation) error
}

func recordMCPToolHealth(
	ctx context.Context,
	health ToolCallHealth,
	observedBy string,
	server string,
	ok bool,
	code string,
	observedAt time.Time,
) {
	if health == nil || server == "" {
		return
	}
	code = MCPMetricCode(code)
	if ok {
		code = CodeMCPNoCode
	}
	err := health.RecordToolCall(ctx, domain.IntegrationToolCallObservation{
		Name:       server,
		OK:         ok,
		Code:       code,
		ObservedAt: observedAt.UTC(),
		ObservedBy: observedBy,
	})
	if err != nil {
		slog.Warn("could not record MCP tool-call health", "server", server, "err", err)
	}
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
