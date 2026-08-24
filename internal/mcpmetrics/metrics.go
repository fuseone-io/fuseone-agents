package mcpmetrics

import "sort"

const (
	CodeOther                     = "other"
	CodeNoCode                    = "none"
	CodeInvokeError               = "invoke_error"
	CodeToolError                 = "tool_error"
	CodeCacheHit                  = "cache_hit"
	CodeUnknownTool               = "unknown_tool"
	CodeUnknownServer             = "unknown_server"
	CodePersonalCredentialMissing = "mcp_personal_credential_missing"
	CodePersonalCredentialRead    = "mcp_personal_credential_read_failed"
	CodePersonalCredentialInvalid = "mcp_personal_credential_invalid"
	CodePersonalCredentialCaller  = "mcp_personal_credential_no_principal"
	CodeServerRateLimited         = "mcp_server_rate_limited"
)

var codes = map[string]bool{
	CodeCacheHit:                  true,
	CodeInvokeError:               true,
	CodeNoCode:                    true,
	CodePersonalCredentialCaller:  true,
	CodePersonalCredentialInvalid: true,
	CodePersonalCredentialMissing: true,
	CodePersonalCredentialRead:    true,
	CodeServerRateLimited:         true,
	CodeToolError:                 true,
	CodeUnknownServer:             true,
	CodeUnknownTool:               true,
}

// Code bounds a failure code before it can become a metric label or UI bucket.
func Code(code string) string {
	if codes[code] {
		return code
	}
	return CodeOther
}

// Codes returns the stable vocabulary used by metrics and UI.
func Codes() []string {
	out := make([]string, 0, len(codes)+1)
	for code := range codes {
		out = append(out, code)
	}
	out = append(out, CodeOther)
	sort.Strings(out)
	return out
}
