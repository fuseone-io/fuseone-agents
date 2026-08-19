import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SessionGate } from "@/features/session/session-gate";

function serve(routes: Record<string, { status: number; body?: unknown }>) {
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const url = typeof input === "string" ? input : input.url;
    const path = new URL(url, "http://localhost").pathname;
    const route = routes[path] ?? { status: 404 };
    return new Response(JSON.stringify(route.body ?? {}), {
      status: route.status,
      headers: { "Content-Type": "application/json" },
    });
  });
}

function renderGate() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <SessionGate>
        <p>console</p>
      </SessionGate>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("what a visitor sees before there is a session", () => {
  it("offers setup while nobody has claimed the installation", async () => {
    serve({
      "/auth/providers": {
        status: 200,
        body: { providers: [], bootstrapPending: true, authRequired: true },
      },
      "/api/v1/me": { status: 401 },
    });
    renderGate();

    expect(
      await screen.findByText(/Configurar esta instalação/),
    ).toBeInTheDocument();
  });

  it("asks the signed-out visitor to sign in once the installation is claimed", async () => {
    serve({
      "/auth/providers": {
        status: 200,
        body: { providers: [], bootstrapPending: false, authRequired: true },
      },
      "/api/v1/me": { status: 401 },
    });
    renderGate();

    // Not a spinner forever: an installation with no identity provider still
    // has to say what is wrong, or nobody can tell it apart from a hung page.
    expect(
      await screen.findByText(/Nenhum provedor de identidade/),
    ).toBeInTheDocument();
  });

  it("renders the console when the server has no identity to sign in to", async () => {
    // A lock on an open door stops nobody and leaves the console unreachable.
    serve({
      "/auth/providers": {
        status: 200,
        body: { providers: [], bootstrapPending: false, authRequired: false },
      },
      "/api/v1/me": { status: 404 },
    });
    renderGate();

    expect(await screen.findByText("console")).toBeInTheDocument();
  });

  it("shows the console to a signed-in caller", async () => {
    serve({
      "/auth/providers": {
        status: 200,
        body: { providers: [], bootstrapPending: false, authRequired: true },
      },
      "/api/v1/me": {
        status: 200,
        body: {
          id: "usr_1",
          display: "Marina",
          kind: "user",
          grants: [],
          can: [],
        },
      },
    });
    renderGate();

    expect(await screen.findByText("console")).toBeInTheDocument();
  });

  it("does not treat a transient /me failure as a signed-out visitor", async () => {
    serve({
      "/auth/providers": {
        status: 200,
        body: { providers: [], bootstrapPending: false, authRequired: true },
      },
      "/api/v1/me": { status: 503 },
    });
    renderGate();

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "A requisição falhou.",
    );
    expect(screen.queryByText(/Nenhum provedor de identidade/)).not.toBeInTheDocument();
  });
});
