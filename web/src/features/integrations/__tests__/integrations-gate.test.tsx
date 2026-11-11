import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import ConsoleRoutes from "@/routes/console-routes";

// The integrations are listed and configured at the platform's scope only. An
// administrator of one area who follows a link there must read why, not a page
// of requests answering 403.

function signedInAs(platform: string[]) {
  const me = {
    id: "usr_ana", display: "Ana", kind: "human",
    can: ["tool:read", "scope:write"], platform, grants: [],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.endsWith("/api/v1/me") ? me : { items: [], mcpServers: [], providers: [] };
      return new Response(JSON.stringify(body), {
        status: 200, headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

afterEach(() => vi.unstubAllGlobals());

// The screens are lazy chunks. Alone a test finds them inside the default
// second; beside the rest of the suite the import alone can take longer.
const LAZY_SCREEN = 5_000;

function openAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <ConsoleRoutes />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("integrations for an administrator of one area", () => {
  it("say they are configured by the platform's administration", async () => {
    signedInAs([]);
    openAt("/integrations/connectors");

    expect(
      await screen.findByText(
        "As integrações são configuradas pela administração da plataforma",
        {},
        { timeout: LAZY_SCREEN },
      ),
    ).toBeInTheDocument();
  });

  it("open as usual for someone who holds tool:read at the platform", async () => {
    signedInAs(["tool:read"]);
    openAt("/integrations");

    expect(
      await screen.findByRole(
        "heading",
        { level: 1, name: "Integrações" },
        { timeout: LAZY_SCREEN },
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("As integrações são configuradas pela administração da plataforma"),
    ).not.toBeInTheDocument();
  });
});
