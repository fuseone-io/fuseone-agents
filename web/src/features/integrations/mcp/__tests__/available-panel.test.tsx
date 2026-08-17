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
  transport: "http",
  url: "https://mcp.stripe.com",
  docs: "https://docs.stripe.com/",
  note: "Payments and billing.",
};

function open() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AvailableServersPanel
          servers={[]}
          recipes={[stripe]}
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
    expect(screen.getByDisplayValue("stripe")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://mcp.stripe.com")).toBeInTheDocument();
  });
});
