import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationsPage } from "@/features/integrations/integrations-page";

const answers: Record<string, unknown> = {
  "/admin/integrations": { mcpServers: [], providers: [], presets: [] },
  "/admin/integrations/connectors/catalog": {
    items: [{ name: "sql", title: "SQL" }, { name: "vault", title: "Vault" }],
  },
};

function answer(path: string): Response {
  if (path === "/me") return new Response(null, { status: 404 });
  if (path in answers) return Response.json(answers[path]);
  return Response.json({ items: [] });
}

describe("the integrations rail", () => {
  afterEach(() => vi.unstubAllGlobals());

  // Counted when the page opens, not only once the connectors are clicked.
  it("counts the connectors on the section the page opens on", async () => {
    vi.stubGlobal("fetch", async (input: Request | string) =>
      answer(new URL(typeof input === "string" ? input : input.url, "http://localhost").pathname.replace(/^\/api\/v1/, "")));
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter>
          <IntegrationsPage section="connected" />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    const rail = await screen.findByRole("navigation", { name: /seções|sections/i });
    const link = within(rail).getByRole("link", { name: /Connectors/ });
    expect(await within(link).findByText("2")).toBeInTheDocument();
  });
});
