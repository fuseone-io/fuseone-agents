import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  useConnectorInstance,
  type ConnectorInstance,
} from "@/features/integrations/api";

afterEach(() => vi.unstubAllGlobals());

describe("connector instance detail", () => {
  it("reads the exact scoped instance before editing it", async () => {
    let request: Request | undefined;
    vi.stubGlobal("fetch", async (next: Request) => {
      request = next;
      return new Response(JSON.stringify({
        name: "app-x",
        connector: "sql",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        hasToken: false,
        sql: {
          driver: "postgres",
          host: "db.internal",
          port: 5432,
          database: "appx",
          credentialSource: {
            kind: "vault_database_role",
            vaultInstance: "prod",
            mount: "database",
            role: "readonly",
          },
          templates: [],
        },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    const instance: ConnectorInstance = {
      name: "app-x", connector: "sql", enabled: true,
      scopeKind: "area", company: "acme", area: "platform", hasToken: false,
    };
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useConnectorInstance(instance), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(request?.method).toBe("GET");
    const url = new URL(request!.url);
    expect(url.pathname).toBe("/api/v1/admin/integrations/connectors/instances/app-x");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      scopeKind: "area",
      company: "acme",
      area: "platform",
    });
    expect(result.current.data?.sql?.templates).toEqual([]);
  });
});
