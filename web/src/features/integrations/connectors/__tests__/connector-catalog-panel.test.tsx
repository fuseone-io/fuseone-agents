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

const runtimeSQL: GovernedConnector = { ...planned, maturity: "runtime" };

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

const sqlInstance: ConnectorInstance = {
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
      role: "app-x-readonly",
    },
    templates: [{
      id: "orders_by_customer",
      parameters: [],
      timeoutSeconds: 10,
      maxRows: 100,
      maxBytes: 65536,
    }],
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

  it("does not open the Vault form for a runtime SQL connector", () => {
    render(
      <ConnectorCatalogPanel
        data={{
          connectors: [vault, runtimeSQL],
          instances: [sqlInstance],
          catalogLoading: false,
          instancesLoading: false,
          catalogError: null,
          instancesError: null,
        }}
        actions={actions()}
      />,
    );

    expect(screen.getAllByText("Executável")).toHaveLength(2);
    expect(screen.getAllByRole("button", { name: "Configurar" })).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();
    expect(screen.getByText("db.internal:5432/appx")).toBeInTheDocument();
    expect(screen.getByText("prod/app-x-readonly")).toBeInTheDocument();
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
