import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConnectorCatalogPanel } from "@/features/integrations/connectors/connector-catalog-panel";
import type { GovernedConnector } from "@/features/integrations/api";

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
    },
  ],
};

describe("governed connector catalogue", () => {
  it("shows connector contracts without offering to configure an instance", () => {
    render(
      <ConnectorCatalogPanel
        connectors={[vault]}
        isLoading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Vault secret storage" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Executável")).toBeInTheDocument();
    expect(screen.getByText("segredos por referência")).toBeInTheDocument();
    expect(screen.getByText("política")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /conectar/i })).toBeNull();
  });
});
