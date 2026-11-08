import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderBare, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectorInstance } from "@/features/integrations/api";
import { ConnectorInstancesPanel } from "@/features/integrations/connectors/connector-instances-panel";
import { setLocale } from "@/i18n";

// The cards read a revision before a removal is confirmed, so they need a client.
const render = (ui: ReactElement) =>
  renderBare(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);

const evidence: ConnectorInstance = {
  name: "gateway",
  connector: "evidence",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "cyber",
  hasToken: true,
  evidence: { baseUrl: "https://source.example.com", queries: 2 },
};

beforeEach(() => setLocale("pt-BR"));

describe("ConnectorInstancesPanel", () => {
  it("offers editing an evidence source to someone who can configure", () => {
    render(
      <ConnectorInstancesPanel
        view={{
          instances: [evidence],
          isLoading: false,
          error: null,
          canConfigure: true,
        }}
        actions={{
          retry: vi.fn(),
          create: vi.fn(),
          edit: vi.fn(),
          remove: vi.fn(),
        }}
      />,
    );

    expect(screen.getByRole("button", { name: "Editar" })).toBeInTheDocument();
  });
});
