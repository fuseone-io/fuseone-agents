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
import { setLocale } from "@/i18n";

afterEach(() => vi.unstubAllGlobals());

beforeEach(() => setLocale("pt-BR"));

describe("connector instance editor", () => {
  // The query and each editor intentionally fail closed on missing detail.
  // This test accuses the query sentinel; the local guards keep rendering safe
  // without relying on TanStack or on this first boundary remaining unchanged.
  it.each<ConfigurableConnector>(["gravitee", "sql"])(
    "refuses an empty %s detail instead of opening a creation form",
    async (connector) => {
      vi.stubGlobal("fetch", async () => new Response(null, {
        status: 200,
        headers: { "Content-Length": "0" },
      }));
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
      expect(screen.queryByLabelText("Nome da instância")).not.toBeInTheDocument();
      expect(client.getQueryState(integrationKeys.connectorInstance(instance))?.error)
        .toMatchObject({ message: "connector instance detail is missing" });
    },
  );
});
