import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentEditorPage } from "@/features/agents/agent-editor-page";

/*
What an author picks from is asked for the agent's own scope (NT-019, rule 5).

Without the target, an area's author is answered the installation's tools and
providers alone, and the tools of their own area's servers never reach the
picker — the agent cannot be given what it is allowed to run.
*/

const AGENT = {
  agent: {
    agentId: "suporte",
    versionId: "vb435fd91",
    scope: { company: "acme", area: "cx" },
    name: "Atendimento",
    provider: "openai",
    model: "devstack",
    tools: [],
    budget: { micros: 500_000, steps: 60 },
    triggers: [],
    publishedAt: "2026-08-14T12:00:00Z",
    latest: true,
    stage: "draft",
  },
  instructions: "",
  versions: [],
};

const answer = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

/** Records every URL asked for; `refuse` answers 404 for a path that contains it. */
function stubApi(refuse?: string) {
  const asked: URL[] = [];
  vi.stubGlobal("fetch", async (input: Request) => {
    const url = new URL(input.url);
    asked.push(url);
    if (refuse && url.pathname.includes(refuse)) {
      return answer({ title: "Not Found", status: 404 }, 404);
    }
    if (url.pathname.endsWith("/agents/suporte")) return answer(AGENT);
    return answer({ items: [] });
  });
  return asked;
}

function openEditor(path = "/agents/suporte/edit") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/agents/new" element={<AgentEditorPage />} />
          <Route path="/agents/:agentId/edit" element={<AgentEditorPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const scopesAsked = (asked: URL[], path: string) =>
  asked
    .filter((url) => url.pathname.endsWith(path))
    .map((url) => url.searchParams.get("scope"));

describe("the agent editor asks for the agent's scope", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("asks the tool catalogue for the scope of the agent being edited", async () => {
    const asked = stubApi();
    openEditor();

    await waitFor(() =>
      expect(scopesAsked(asked, "/admin/tools")).toContain("acme/cx"),
    );
    // Never the unscoped answer first, which an area's author reads short.
    expect(scopesAsked(asked, "/admin/tools")).not.toContain(null);
  });

  it("asks a new agent's catalogue for no target until an area is chosen", async () => {
    const asked = stubApi();
    openEditor("/agents/new");

    await waitFor(() =>
      expect(scopesAsked(asked, "/admin/tools")).toEqual([null]),
    );
  });

  it("says the area cannot be read when the catalogue answers 404", async () => {
    stubApi("/admin/tools");
    openEditor();

    await userEvent.click(await screen.findByRole("tab", { name: /Ferramentas/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /não pode consultar o que esta área usa/,
    );
  });
});
