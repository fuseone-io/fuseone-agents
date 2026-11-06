import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  integrationKeys,
  type ConnectorInstance,
} from "@/features/integrations/api";
import { ConnectorEditor } from "@/features/integrations/connectors/connector-editor";
import type { ConfigurableConnector } from "@/features/integrations/connectors/connector-editor-registry";
import { evidenceExamples } from "@/features/integrations/connectors/evidence-examples";
import { setLocale } from "@/i18n";

afterEach(() => vi.unstubAllGlobals());

beforeEach(() => setLocale("pt-BR"));

describe("connector instance editor", () => {
  // The query and each editor intentionally fail closed on missing detail.
  // This test accuses the query sentinel; the local guards keep rendering safe
  // without relying on TanStack or on this first boundary remaining unchanged.
  it.each<ConfigurableConnector>(["gravitee", "sql", "evidence"])(
    "refuses an empty %s detail instead of opening a creation form",
    async (connector) => {
      vi.stubGlobal(
        "fetch",
        async () =>
          new Response(null, {
            status: 200,
            headers: { "Content-Length": "0" },
          }),
      );
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      const wrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      );
      const instance: ConnectorInstance = {
        name: "configured",
        connector,
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        hasToken: false,
      };

      render(
        <ConnectorEditor
          connector={connector}
          instance={instance}
          instances={[instance]}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />,
        { wrapper },
      );

      expect(await screen.findByRole("alert")).toBeInTheDocument();
      expect(
        screen.queryByLabelText("Nome da instância"),
      ).not.toBeInTheDocument();
      expect(
        client.getQueryState(integrationKeys.connectorInstance(instance))
          ?.error,
      ).toMatchObject({ message: "connector instance detail is missing" });
    },
  );

  it("edits an evidence source from its detail, not from the listing's summary", async () => {
    const declaration = evidenceExamples[0]!.declaration;
    const summary: ConnectorInstance = {
      name: "gateway",
      connector: "evidence",
      enabled: true,
      scopeKind: "area",
      company: "acme",
      area: "cyber",
      hasToken: true,
      evidence: { baseUrl: declaration.baseUrl, queries: 3 },
    };
    vi.stubGlobal(
      "fetch",
      async () =>
        new Response(JSON.stringify({ ...summary, evidence: declaration }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={client}>
        <ConnectorEditor
          connector="evidence"
          instance={summary}
          instances={[summary]}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("pages_by_user")).toBeInTheDocument();
  });

  it("refuses to edit from a cached declaration when reading the current one fails", async () => {
    const declaration = evidenceExamples[0]!.declaration;
    const summary: ConnectorInstance = {
      name: "gateway",
      connector: "evidence",
      enabled: true,
      scopeKind: "area",
      company: "acme",
      area: "cyber",
      hasToken: true,
      evidence: { baseUrl: declaration.baseUrl, queries: 3 },
    };
    vi.stubGlobal(
      "fetch",
      async () =>
        new Response(JSON.stringify({ detail: "down" }), {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(integrationKeys.connectorInstance(summary), {
      ...summary,
      evidence: declaration,
    });

    render(
      <QueryClientProvider client={client}>
        <ConnectorEditor
          connector="evidence"
          instance={summary}
          instances={[summary]}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Nome da instância"),
    ).not.toBeInTheDocument();
  });

  it("opens a new evidence source without asking the server for anything", () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    render(
      <QueryClientProvider client={new QueryClient()}>
        <ConnectorEditor
          connector="evidence"
          instance={null}
          instances={[]}
          onClose={vi.fn()}
          onSave={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByLabelText("Nome da instância")).toBeEnabled();
    expect(fetch).not.toHaveBeenCalled();
  });
});
