import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConnectorCatalogPanel } from "@/features/integrations/connectors/connector-catalog-panel";
import type {
  ConnectorInstance,
  GovernedConnector,
} from "@/features/integrations/api";

const vault: GovernedConnector = {
  id: "vault",
  name: "Vault secret storage",
  category: "secrets",
  summary: "Store generated keys and certificates without returning secrets.",
  maturity: "runtime",
  guarantees: [
    "secret values are written from content references, not inline model text",
    "paths are constrained by connector policy before a write reaches Vault",
  ],
  caveats: ["does not generate cryptographic material by itself"],
  operations: [
    {
      id: "vault.write_secret",
      name: "Write secret",
      summary: "Writes a generated key or certificate to an allowed Vault path.",
      effects: ["write"],
      approval: "policy",
      secretHandling: "reference_only",
    cachePolicy: "never",
    },
  ],
};

const planned: GovernedConnector = {
  ...vault,
  id: "sql",
  name: "Governed SQL",
  maturity: "planned",
  operations: [
    {
      id: "sql.query_template",
      name: "Run template",
      summary: "Runs a declared read-only SQL template.",
      effects: ["read"],
      approval: "policy",
      secretHandling: "reference_only",
    cachePolicy: "never",
    },
  ],
};

const instance: ConnectorInstance = {
  name: "prod",
  connector: "vault",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: true,
  updatedAt: "2026-08-25T12:00:00Z",
  vault: {
    address: "https://vault.example.com",
    mount: "secret",
    allowedPathPrefixes: ["certificates"],
  },
};

function actions() {
  return {
    retryCatalog: vi.fn(),
    retryInstances: vi.fn(),
    saveInstance: vi.fn(),
    deleteInstance: vi.fn(),
  };
}

describe("governed connector catalogue", () => {
  it("separates executable runtime connectors from planned shapes", () => {
    render(
      <ConnectorCatalogPanel
        data={{
          connectors: [vault, planned],
          instances: [],
          catalogLoading: false,
          instancesLoading: false,
          catalogError: null,
          instancesError: null,
        }}
        actions={actions()}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Vault secret storage" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Executável")).toBeInTheDocument();
    expect(screen.getByText("Planejado")).toBeInTheDocument();
    expect(screen.getAllByText("segredos por referência")).toHaveLength(2);
    expect(screen.getAllByText("política")).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Configurar" })).toHaveLength(
      1,
    );
  });

  it("shows configured instances as executable endpoints", () => {
    render(
      <ConnectorCatalogPanel
        data={{
          connectors: [vault],
          instances: [instance],
          catalogLoading: false,
          instancesLoading: false,
          catalogError: null,
          instancesError: null,
        }}
        actions={actions()}
      />,
    );

    expect(screen.getByRole("heading", { name: "prod" })).toBeInTheDocument();
    expect(screen.getByText("vault em acme/platform")).toBeInTheDocument();
    expect(screen.getByText("selado")).toBeInTheDocument();
    expect(screen.getByText("certificates")).toBeInTheDocument();
  });

  it("keeps the catalogue visible when configured instances fail to load", () => {
    render(
      <ConnectorCatalogPanel
        data={{
          connectors: [vault],
          instances: [],
          catalogLoading: false,
          instancesLoading: false,
          catalogError: null,
          instancesError: new Error("instances failed"),
        }}
        actions={actions()}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Vault secret storage" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("keeps configured instances visible when the catalogue fails to load", () => {
    render(
      <ConnectorCatalogPanel
        data={{
          connectors: [],
          instances: [instance],
          catalogLoading: false,
          instancesLoading: false,
          catalogError: new Error("catalogue failed"),
          instancesError: null,
        }}
        actions={actions()}
      />,
    );

    expect(screen.getByRole("heading", { name: "prod" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });
});
