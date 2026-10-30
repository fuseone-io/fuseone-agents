import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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

function openAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <ConsoleRoutes />
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
