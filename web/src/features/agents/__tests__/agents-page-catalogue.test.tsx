import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentsPage } from "@/features/agents/agents-page";
import { usePreferences } from "@/features/preferences/use-preferences";
import { setLocale } from "@/i18n";
import { json } from "@/test/revised-routes";

/*
Each card reads its agent's tools from the answer for that agent's scope
(NT-019). Unscoped, an area's reader is answered the installation's tools
alone, and a card would call its area's writing tool unclassified.
*/

const agent = (agentId: string, area: string, tool: string) => ({
  agentId, versionId: `v-${agentId}`, scope: { company: "acme", area }, name: agentId,
  provider: "openai", model: "m", tools: [tool], budget: { micros: 1, steps: 1 },
  publishedAt: "2026-08-11T12:00:00Z", latest: true,
});
const BY_SCOPE: Record<string, unknown[]> = {
  "acme/cx": [{ toolId: "crm.reply", server: "crm", effect: "write", untrusted: false }],
  "acme/fin": [{ toolId: "ledger.read", server: "ledger", effect: "read", untrusted: false }],
};

function stubApi() {
  const scopes: (string | null)[] = [];
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const url = new URL(typeof input === "string" ? input : input.url, "http://localhost");
    if (url.pathname.endsWith("/agents")) {
      return json({ items: [agent("triage", "cx", "crm.reply"), agent("books", "fin", "ledger.read")] });
    }
    if (!url.pathname.endsWith("/admin/tools")) return json({ items: [] });
    scopes.push(url.searchParams.get("scope"));
    return json({ items: BY_SCOPE[url.searchParams.get("scope") ?? ""] ?? [] });
  });
  return scopes;
}

beforeEach(() => {
  setLocale("pt-BR");
  usePreferences.setState({ agentsView: "cards" });
});
afterEach(() => vi.unstubAllGlobals());

describe("the agents list's cards", () => {
  it("ask once per distinct scope and read each card from its own", async () => {
    const scopes = stubApi();
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter><AgentsPage /></MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText(/1 pode escrever/)).toBeInTheDocument();
    expect(await screen.findByText(/Somente leitura/)).toBeInTheDocument();
    await waitFor(() => expect([...scopes].sort()).toEqual(["acme/cx", "acme/fin"]));
  });
});
