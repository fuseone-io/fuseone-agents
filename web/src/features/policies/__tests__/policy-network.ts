import { vi } from "vitest";
import type { Policy } from "@/lib/api/client";

export type Recorded = {
  method: string;
  path: string;
  ifMatch: string | null;
  body?: unknown;
};

export function aPolicy(over: Partial<Policy> = {}): Policy {
  return {
    code: "POL-200",
    name: "Guard exports",
    resource: "*",
    effects: [],
    conditions: [],
    effect: "deny",
    mode: "enforce",
    enabled: true,
    ...over,
  };
}

/**
 * The policy routes at the network boundary, judging If-Match the way the
 * server does: every write moves the revision, and a stale one gets a 412.
 */
export function stubPolicies(start: Policy[] = [aPolicy()]) {
  const state = { policies: start, generation: 1 };
  const requests: Recorded[] = [];
  const etag = () => `"v1-gen${state.generation}"`;
  const json = (
    body: unknown,
    status = 200,
    headers: Record<string, string> = {},
  ) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json", ...headers },
    });
  const stale = () =>
    json(
      { type: "fuseone:precondition-failed", title: "stale", status: 412 },
      412,
      { ETag: etag() },
    );

  vi.stubGlobal("fetch", async (input: Request | string) => {
    const request =
      typeof input === "string"
        ? new Request(new URL(input, "http://localhost"))
        : input;
    const path = new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    const ifMatch = request.headers.get("If-Match");
    const body = request.method === "PUT" ? await request.json() : undefined;
    requests.push({ method: request.method, path, ifMatch, body });
    if (path === "/me") return new Response(null, { status: 404 });
    if (path === "/policies") return json({ items: state.policies });
    const code = decodeURIComponent(path.replace("/policies/", ""));
    const found = state.policies.find((p) => p.code === code);
    if (request.method === "GET")
      return found ? json(found, 200, { ETag: etag() }) : json({}, 404);
    if (ifMatch && ifMatch !== etag()) return stale();
    state.generation++;
    state.policies = state.policies.filter((p) => p.code !== code);
    if (request.method === "PUT")
      state.policies.push({ ...(body as Policy), code });
    return request.method === "PUT"
      ? json({ ...(body as Policy), code }, 200, { ETag: etag() })
      : new Response(null, { status: 204 });
  });

  /** Somebody else writes, moving the revision under the screen. */
  const elsewhere = (over: Partial<Policy>) => {
    state.generation++;
    state.policies = state.policies.map((p) =>
      p.code === (over.code ?? p.code) ? { ...p, ...over } : p,
    );
  };
  return { state, requests, elsewhere, etag };
}
