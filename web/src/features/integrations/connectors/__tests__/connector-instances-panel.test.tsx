import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ConnectorInstance } from "@/features/integrations/api";
import { ConnectorInstancesPanel } from "@/features/integrations/connectors/connector-instances-panel";
import { setLocale } from "@/i18n";

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
