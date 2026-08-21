export const FAILURE_LABELS: Record<string, string> = {
  model_provider_overloaded: "runtime.failureModelProviderOverloaded",
  model_rate_limited: "runtime.failureModelRateLimited",
  model_auth_failed: "runtime.failureModelAuthFailed",
  model_bad_request: "runtime.failureModelBadRequest",
  model_provider_unavailable: "runtime.failureModelProviderUnavailable",
  model_network: "runtime.failureModelNetwork",
  model_refused: "runtime.failureModelRefused",
  model_provider_error: "runtime.failureModelProviderError",
  mcp_server_rate_limited: "runtime.failureMCPServerRateLimited",
  attempts_exhausted: "runtime.failureAttemptsExhausted",
};

export function failureLabel(code: string): string {
  return FAILURE_LABELS[code] ?? code;
}
