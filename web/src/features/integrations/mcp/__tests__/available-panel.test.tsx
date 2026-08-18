import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { AvailableServersPanel } from "@/features/integrations/mcp/available-servers-panel";
import type { ServerRecipe } from "@/features/integrations/mcp/api";

const stripe: ServerRecipe = {
  server: "stripe",
  title: "Stripe",
  category: "finance",
  publisher: "Stripe",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "published",
  configRequirements: ["credential"],
  authModes: [{ type: "oauth2", principal: "user", label: "Stripe OAuth" }],
  transport: "http",
  url: "https://mcp.stripe.com",
  docs: "https://docs.stripe.com/",
  note: "Payments and billing.",
};

const postgres: ServerRecipe = {
  server: "postgres",
  title: "PostgreSQL",
  category: "data",
  publisher: "Model Context Protocol reference servers",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "archived",
  configRequirements: ["credential"],
  authModes: [{ type: "dsn", principal: "service", label: "PostgreSQL DSN" }],
  note: "Read-only database access.",
};

const datadog: ServerRecipe = {
  server: "datadog",
  title: "Datadog",
  category: "operations",
  publisher: "Datadog",
  docsFrom: "publisher",
  provenance: "documentation",
  status: "published",
  configRequirements: ["credential"],
  authModes: [
    {
      type: "headers",
      principal: "service",
      label: "API and application key headers",
    },
  ],
  transport: "http",
  url: "https://mcp.datadoghq.com/api/unstable/mcp-server/mcp",
  note: "Observability.",
};

function open(recipes: ServerRecipe[] = [stripe]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AvailableServersPanel
          servers={[]}
          recipes={recipes}
          isLoading={false}
          error={null}
          onRetry={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/*
The catalogue card opens the configuration panel beside the list.

Routing to `/integrations/mcp/new?recipe=stripe` lost the user's place in the
catalogue and put the form under another grid of recipes. The handoff's shape
is more specific: the selected server stays in view and the right panel holds
the connection act for exactly that server.
*/
describe("available MCP servers", () => {
  it("opens the selected recipe in the side configuration panel", async () => {
    const { container } = open();

    expect(
      container.querySelector('[data-mcp-icon="stripe"] svg'),
    ).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Stripe" }),
    );

    expect(screen.getByRole("heading", { name: "Stripe" })).toBeInTheDocument();
    expect(screen.getAllByText("Stripe OAuth").length).toBeGreaterThan(0);
    expect(screen.getByDisplayValue("stripe")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://mcp.stripe.com")).toBeInTheDocument();
    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/access token oauth/i)).toBeInTheDocument();
  });

  it("does not let an archived reference recipe look current", async () => {
    const { container } = open([postgres]);

    expect(screen.getByText("arquivado")).toBeInTheDocument();
    expect(screen.getByText("credencial")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar PostgreSQL" }),
    );

    expect(screen.getAllByText("Read-only database access.")).toHaveLength(2);
    expect(
      screen.getByText(/marcou este servidor como arquivado/),
    ).toBeInTheDocument();
    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
  });

  it("does not turn a multi-header recipe into one token field", async () => {
    const { container } = open([datadog]);

    await userEvent.click(
      screen.getByRole("button", { name: "Conectar Datadog" }),
    );

    expect(container.querySelector("#token")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token bearer/i)).not.toBeInTheDocument();
    expect(
      screen.getByText(/runtime não configura essa forma/i),
    ).toBeInTheDocument();
  });
});
