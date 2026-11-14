import { vi } from "vitest";
import { scopeListsRoute } from "@/test/scope-lists-routes";
import type {
  ServiceAccount,
  ServiceToken,
} from "@/features/admin/service-accounts-api";

export const SECRET = "fus_the-one-time-secret";

export type Recorded = { method: string; path: string; body?: unknown };

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

export function anAccount(over: Partial<ServiceAccount> = {}): ServiceAccount {
  return {
    id: "svc_publisher",
    name: "publisher",
    description: "publishes from CI",
    disabled: false,
    createdAt: "2026-11-01T12:00:00Z",
    grants: [{ company: "acme", area: "cx", role: "author" }],
    ...over,
  };
}

export function aToken(over: Partial<ServiceToken> = {}): ServiceToken {
  return {
    id: "tok_old",
    name: "ci",
    createdBy: "usr_admin",
    createdAt: "2026-11-01T12:00:00Z",
    expiresAt: "2099-01-01T00:00:00Z",
    ...over,
  };
}

/**
 * The routes, at the network boundary, keeping state the way the server does:
 * a screen that forgets to refetch shows the old answer, as it would for real.
 */
export function stubAccounts(
  start: { accounts?: ServiceAccount[]; tokens?: ServiceToken[] } = {},
) {
  const state = {
    accounts: start.accounts ?? [anAccount()],
    tokens: start.tokens ?? [aToken()],
    /** Set to hold or refuse the next reads of the token list. */
    tokensRead: undefined as (() => Promise<Response>) | undefined,
  };
  const requests: Recorded[] = [];

  type Body = { name: string; description?: string } & {
    disabled: boolean;
    expiresAt: string;
    grants: ServiceAccount["grants"];
  };
  const route = (method: string, path: string, body: Body): Response => {
    const listed = scopeListsRoute(method, path);
    if (listed) return listed;
    if (path === "/admin/service-accounts") {
      if (method === "GET") return json({ items: state.accounts });
      const made = anAccount({
        id: `svc_${body.name}`,
        name: body.name,
        description: body.description,
        grants: [],
      });
      state.accounts = [...state.accounts, made];
      return json(made, 201);
    }
    const at = path.match(/^\/admin\/service-accounts\/[^/]+\/disabled$/);
    if (at) {
      state.accounts = state.accounts.map((a) => ({
        ...a,
        disabled: body.disabled,
      }));
      return new Response(null, { status: 204 });
    }
    if (path.endsWith("/tokens")) {
      if (method === "GET") return json({ items: state.tokens });
      const made = aToken({ id: "tok_new", name: body.name, expiresAt: body.expiresAt });
      state.tokens = [made, ...state.tokens];
      return json({ ...made, secret: SECRET }, 201);
    }
    const revoke = path.match(/\/tokens\/([^/]+)$/);
    if (revoke) {
      state.tokens = state.tokens.map((token) =>
        token.id === revoke[1]
          ? { ...token, revokedAt: "2026-11-07T12:00:00Z" }
          : token,
      );
      return new Response(null, { status: 204 });
    }
    if (path.endsWith("/grants")) {
      state.accounts = state.accounts.map((a) => ({
        ...a,
        grants: body.grants,
      }));
      return new Response(null, { status: 204 });
    }
    throw new Error(`unexpected request: ${method} ${path}`);
  };

  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace("/api/v1", "");
      const text = await request.clone().text();
      const body: unknown = text ? JSON.parse(text) : undefined;
      if (request.method !== "GET") {
        requests.push({ method: request.method, path, body });
      }
      if (request.method === "GET" && path.endsWith("/tokens") && state.tokensRead) {
        return state.tokensRead();
      }
      return route(request.method, path, body as Body);
    }),
  );
  return { requests, state };
}
