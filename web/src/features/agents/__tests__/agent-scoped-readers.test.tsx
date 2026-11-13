import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AgentCapabilities } from "@/features/agents/agent-capabilities";
import { NarrativeCard } from "@/features/agents/narrative-card";
import { json } from "@/test/revised-routes";
import type { Agent, AgentDefinition } from "@/lib/api/client";

/*
What an existing agent's screens say about its tools is read at the agent's
own scope (NT-019). Unscoped, an area's reader is answered the installation's
tools alone, and the area's own tools read as nobody having ruled on them.
*/

const AREA_TOOL = { toolId: "crm.lookup", server: "crm", effect: "read", untrusted: false };

function stubTools() {
  const scopes: (string | null)[] = [];
  vi.stubGlobal("fetch", async (input: Request | string) => {
    const url = new URL(typeof input === "string" ? input : input.url, "http://localhost");
    if (!url.pathname.endsWith("/admin/tools")) return json({ items: [] });
    scopes.push(url.searchParams.get("scope"));
    return json({ items: url.searchParams.get("scope") === "acme/cx" ? [AREA_TOOL] : [] });
  });
  return scopes;
}

function show(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>);
}

const agent = {
  agentId: "suporte", versionId: "vb435fd91", scope: { company: "acme", area: "cx" },
  name: "Atendimento", provider: "openai", model: "devstack", tools: ["crm.lookup"],
  budget: { micros: 500_000, steps: 60 }, publishedAt: "2026-08-13T21:56:57Z", latest: true,
} as Agent;

afterEach(() => vi.unstubAllGlobals());

describe("an existing agent's tools, read at its scope", () => {
  it("the capabilities card shows the effect the agent's area ruled", async () => {
    const scopes = stubTools();
    show(<AgentCapabilities agent={agent} />);

    expect(await screen.findByText(/Leitura/i)).toBeInTheDocument();
    expect(scopes).toEqual(["acme/cx"]);
  });

  it("the narrative card asks for the draft's scope", async () => {
    const scopes = stubTools();
    const draft = { name: "A", company: "acme", area: "cx", tools: ["crm.lookup"], budget: {}, triggers: [] };
    show(<NarrativeCard draft={draft as unknown as AgentDefinition} />);

    await waitFor(() => expect(scopes).toContain("acme/cx"));
    expect(scopes).not.toContain(null);
  });
});
