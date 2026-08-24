export const FAILURE_LABELS: Record<string, string> = {
  model_provider_overloaded: "runtime.failureModelProviderOverloaded",
  model_rate_limited: "runtime.failureModelRateLimited",
  model_auth_failed: "runtime.failureModelAuthFailed",
  model_bad_request: "runtime.failureModelBadRequest",
  model_provider_unavailable: "runtime.failureModelProviderUnavailable",
  model_network: "runtime.failureModelNetwork",
  model_refused: "runtime.failureModelRefused",
  model_provider_error: "runtime.failureModelProviderError",
  invoke_error: "runtime.failureMCPInvokeError",
  mcp_personal_credential_invalid: "runtime.failureMCPPersonalCredentialInvalid",
  mcp_personal_credential_missing: "runtime.failureMCPPersonalCredentialMissing",
  mcp_personal_credential_no_principal: "runtime.failureMCPPersonalCredentialCaller",
  mcp_personal_credential_read_failed: "runtime.failureMCPPersonalCredentialRead",
  mcp_server_rate_limited: "runtime.failureMCPServerRateLimited",
  none: "runtime.failureMCPNoCode",
  other: "runtime.failureOther",
  tool_error: "runtime.failureMCPToolError",
  unknown_server: "runtime.failureMCPUnknownServer",
  unknown_tool: "runtime.failureMCPUnknownTool",
  attempts_exhausted: "runtime.failureAttemptsExhausted",
};

export function failureLabel(code: string): string {
  return FAILURE_LABELS[code] ?? code;
}
