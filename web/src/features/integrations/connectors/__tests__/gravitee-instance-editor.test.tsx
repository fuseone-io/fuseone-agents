import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  integrationKeys,
  type ConnectorInstance,
} from "@/features/integrations/api";
import { ConnectorEditor } from "@/features/integrations/connectors/connector-editor";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

const listed: ConnectorInstance = {
  name: "apim",
  connector: "gravitee",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: false,
};

const vault: ConnectorInstance = {
  name: "secrets",
  connector: "vault",
  enabled: true,
  scopeKind: "company",
  company: "acme",
  hasToken: true,
  vault: {
    address: "https://vault.internal",
    mount: "secret",
    allowedPathPrefixes: ["integrations/gravitee"],
  },
};

afterEach(() => vi.unstubAllGlobals());

beforeEach(() => {
  setLocale("pt-BR");
  useActiveScope.setState({ company: "acme", area: "platform" });
});

describe("Gravitee connector instance editor", () => {
  it("loads the configurer-only detail before mounting the edit form", async () => {
    let request: Request | undefined;
    vi.stubGlobal("fetch", async (next: Request) => {
      request = next;
      return new Response(JSON.stringify({
        ...listed,
        gravitee: {
          address: "https://apim.internal/management/v2",
          organization: "org-prod",
          environment: "env-prod",
          allowedReferences: [{ type: "API", id: "checkout-api" }],
          minTTLSeconds: 86_400,
          maxTTLSeconds: 7_776_000,
          allowNoExpiry: false,
          credentialSource: {
            kind: "vault_kv_secret",
            vaultInstance: "secrets",
            path: "integrations/gravitee/prod",
            field: "access_token",
          },
        },
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );

    render(
      <ConnectorEditor
        connector="gravitee"
        instance={listed}
        instances={[listed, vault]}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
      { wrapper },
    );

    expect(screen.queryByLabelText("Caminho do segredo")).not.toBeInTheDocument();
    expect(await screen.findByLabelText("Caminho do segredo")).toHaveValue(
      "integrations/gravitee/prod",
    );
    await waitFor(() => expect(request).toBeDefined());
    const url = new URL(request!.url);
    expect(url.pathname).toBe("/api/v1/admin/integrations/connectors/instances/apim");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      scopeKind: "area",
      company: "acme",
      area: "platform",
    });
  });

  it("waits for a fresh detail instead of mounting a cached credential boundary", async () => {
    let release!: (response: Response) => void;
    const fetcher = vi.fn(() => new Promise<Response>((resolve) => {
      release = resolve;
    }));
    vi.stubGlobal("fetch", fetcher);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    client.setQueryData(integrationKeys.connectorInstance(listed), {
      ...listed,
      gravitee: graviteeDetail("credentials/old"),
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );

    render(
      <ConnectorEditor
        connector="gravitee"
        instance={listed}
        instances={[listed, vault]}
        onClose={vi.fn()}
        onSave={vi.fn()}
      />,
      { wrapper },
    );

    expect(screen.queryByLabelText("Caminho do segredo")).not.toBeInTheDocument();
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
    await act(async () => {
      release(new Response(JSON.stringify({
        ...listed,
        gravitee: graviteeDetail("integrations/gravitee/current"),
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    });
    expect(await screen.findByLabelText("Caminho do segredo")).toHaveValue(
      "integrations/gravitee/current",
    );
  });
});

function graviteeDetail(path: string) {
  return {
    address: "https://apim.internal/management/v2",
    organization: "org-prod",
    environment: "env-prod",
    allowedReferences: [{ type: "API" as const, id: "checkout-api" }],
    minTTLSeconds: 86_400,
    maxTTLSeconds: 7_776_000,
    allowNoExpiry: false,
    credentialSource: {
      kind: "vault_kv_secret" as const,
      vaultInstance: "secrets",
      path,
      field: "access_token",
    },
  };
}
