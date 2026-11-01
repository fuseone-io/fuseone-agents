import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { AgentEditorPage } from "@/features/agents/agent-editor-page";

/*
Publishing a version whose events come back to start it (#227).

The version is written either way — a review loop is a cycle on purpose — so
what the author has to see is the warning the answer carries, naming every
agent in the loop, and nothing extra when the answer carries none.
*/

const AGENT = {
  agent: {
    agentId: "developer",
    versionId: "vb435fd91",
    scope: { company: "acme", area: "factory" },
    name: "Developer",
    provider: "openai",
    model: "devstack",
    tools: ["crm.lookup"],
    budget: { micros: 500_000, steps: 60 },
    triggers: [],
    publishedAt: "2026-08-14T12:00:00Z",
    latest: true,
    stage: "draft",
  },
  instructions: "Escreva o código do card.",
  versions: [],
};

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

/** The server, answering a publish with the warning it was given. */
function serve(eventCycleWarning?: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      if (input.method === "PUT" && input.url.includes("/versions")) {
        return json({
          agentId: "developer",
          versionId: "vc0ffee12",
          created: true,
          paused: true,
          eventCycleWarning,
        });
      }
      return json(
        input.url.includes("/agents/developer") ? AGENT : { items: [] },
      );
    }),
  );
}

async function publish() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/agents/developer/edit"]}>
        <Routes>
          <Route path="/agents/:agentId/edit" element={<AgentEditorPage />} />
          <Route path="/agents/:agentId" element={null} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
  const button = await screen.findByRole("button", { name: /Publicar versão/ });
  await waitFor(() => expect(button).toBeEnabled());
  await userEvent.click(button);
  await screen.findByText(/Versão vc0ffee12 publicada/);
}

describe("publishing a version that closes an event cycle", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("publishes, and warns naming every agent in the cycle", async () => {
    serve({ cycles: [["developer", "reviewer"]], hopLimit: 8 });

    await publish();

    expect(
      await screen.findByText(/developer → reviewer → developer/),
    ).toBeInTheDocument();
    expect(screen.getByText(/8 saltos/)).toBeInTheDocument();
  });

  it("says nothing more when the version closes no cycle", async () => {
    serve();

    await publish();

    expect(screen.queryByText(/ciclo/)).not.toBeInTheDocument();
  });
});
