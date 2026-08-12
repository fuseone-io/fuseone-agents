import createClient from "openapi-fetch";
import type { components, paths } from "./schema.gen";

// The client is generated from api/openapi.yaml, the same file the Go server
// implements. A path, parameter or field that does not exist in the contract
// is a type error here, so drift between the two sides cannot reach runtime.
// Absolute, resolved against the page's own origin. The client builds a
// Request, and the Request constructor rejects a relative URL outside a
// browser — which would make the whole client untestable for no gain.
const baseUrl = new URL(
  "/api/v1",
  globalThis.location?.origin ?? "http://localhost",
).toString();

export const api = createClient<paths>({
  baseUrl,
  // Resolve fetch per call instead of capturing it when the module loads.
  // The captured reference cannot be replaced afterwards, which would force
  // every test to mock this module rather than the network boundary.
  fetch: (request) => globalThis.fetch(request),
});

const CSRF_COOKIE = "fuseone_csrf=";
const CSRF_HEADER = "X-CSRF-Token";
const UNSAFE = new Set(["POST", "PUT", "PATCH", "DELETE"]);

/**
 * The CSRF cookie is deliberately readable by the console's own scripts; the
 * session cookie is not. Echoing one in a header is what proves the request
 * came from a page on this origin rather than from a form somebody else
 * hosted.
 */
export function csrfToken(): string | undefined {
  return globalThis.document?.cookie
    ?.split("; ")
    .find((entry) => entry.startsWith(CSRF_COOKIE))
    ?.slice(CSRF_COOKIE.length);
}

api.use({
  onRequest({ request }) {
    if (!UNSAFE.has(request.method)) return undefined;
    const token = csrfToken();
    if (token) request.headers.set(CSRF_HEADER, token);
    return request;
  },
});

export type Run = components["schemas"]["Run"];
export type Step = components["schemas"]["Step"];
export type StepKind = components["schemas"]["StepKind"];
export type Phase = components["schemas"]["Phase"];
export type PendingApproval = components["schemas"]["PendingApproval"];
export type CostRollup = components["schemas"]["CostRollup"];
export type RunStats = components["schemas"]["RunStats"];
export type Cost = components["schemas"]["Cost"];
export type Agent = components["schemas"]["Agent"];
export type AgentVersion = components["schemas"]["AgentVersion"];
export type ThroughputBucket = components["schemas"]["ThroughputBucket"];
export type RecordedDecision = components["schemas"]["RecordedDecision"];
export type Verdict = components["schemas"]["Verdict"];
export type Tool = components["schemas"]["Tool"];
export type Webhook = components["schemas"]["Webhook"];
export type AuditEntry = components["schemas"]["AuditEntry"];
export type IntegrationHealth = components["schemas"]["IntegrationHealth"];
export type MCPServer = components["schemas"]["MCPServer"];
export type Policy = components["schemas"]["Policy"];
export type PolicyInput = components["schemas"]["PolicyInput"];
export type PolicyCondition = components["schemas"]["PolicyCondition"];
export type Simulation = components["schemas"]["Simulation"];
export type ModelProvider = components["schemas"]["ModelProvider"];
export type Problem = components["schemas"]["Problem"];

/**
 * ApiError carries the RFC 9457 problem body so a view can show what failed
 * and what to do about it, rather than a bare status code.
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly problem?: Problem,
  ) {
    super(problem?.title ?? `Request failed with status ${status}`);
    this.name = "ApiError";
  }

  get detail(): string | undefined {
    return this.problem?.detail;
  }
}

/**
 * unwrap turns openapi-fetch's `{ data, error }` into a value or a throw, so
 * TanStack Query handles failures through its own error channel instead of
 * every caller branching on two shapes.
 */
export function unwrap<T>(result: {
  data?: T;
  error?: unknown;
  response: Response;
}): T {
  if (result.error !== undefined || result.data === undefined) {
    throw new ApiError(result.response.status, result.error as Problem);
  }
  return result.data;
}
