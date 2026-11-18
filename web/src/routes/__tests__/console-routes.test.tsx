import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ConsoleRoutes from "@/routes/console-routes";

// Screens are fetched on demand now. What can go wrong is the wiring: a route
// whose lazy import resolves to nothing, or a fallback that never yields to the
// screen. So a deep link must end on the real screen, not on its skeleton.

const ME = { id: "usr_ana", display: "Ana", kind: "human", can: [], grants: [] };

/** The shell asks who is signed in; every other read comes back empty. */
beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.endsWith("/api/v1/me") ? ME : { items: [], pages: [] };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
});

afterEach(() => vi.unstubAllGlobals());

/** Where the router settled, for a test that follows a redirect. */
function Where() {
  return <output aria-label="location">{useLocation().pathname}</output>;
}

function openAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <ConsoleRoutes />
        <Where />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("the console's lazily loaded screens", () => {
  it("a deep link settles on the screen once its code has arrived", async () => {
    openAt("/manual");

    expect(
      await screen.findByRole("heading", { level: 1, name: "Manual" }),
    ).toBeInTheDocument();
  });
});

/*
Creating and editing an agent go through the workbench (NT-018, delivery R):
the form and the guided interview are no longer where any door leads.
*/
describe("the doors to authoring an agent", () => {
  it("opens the workbench's first step for a new agent", async () => {
    openAt("/agents/new");
    expect(await screen.findByText(/Primeiro passo: identificador, nome e área/)).toBeInTheDocument();
  });

  it.each([
    ["/agents/triagem/edit", "/agents/triagem/workbench"],
    ["/agents/interview", "/agents/new"],
  ])("sends %s to %s", async (from, to) => {
    openAt(from);
    // The redirect may itself be loaded on demand: wait for the router to settle.
    await waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent(new RegExp(`^${to}$`)));
  });

  it("offers one way to create an agent, and it leads to the workbench", async () => {
    openAt("/agents");
    expect(await screen.findByRole("link", { name: /Novo agente/ })).toHaveAttribute("href", "/agents/new");
    expect(screen.queryByRole("link", { name: /Agente guiado/ })).not.toBeInTheDocument();
  });
});
