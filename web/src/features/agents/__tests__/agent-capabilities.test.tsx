import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentCapabilities } from "@/features/agents/agent-capabilities";
import type { Agent } from "@/lib/api/client";

/*
A ceiling nobody set is a sentence, not a key.

Zero means no ceiling, which is a different thing from a ceiling of zero — and
the screen has to say which. The guard that checks every key exists cannot see
this one: the key is real, it was simply never handed to the translator.
*/

const agent: Agent = {
  agentId: "suporte",
  versionId: "vb435fd91",
  scope: { company: "acme", area: "cx" },
  name: "Atendimento",
  provider: "openai",
  model: "devstack",
  tools: ["crm.lookup"],
  budget: { micros: 500_000, steps: 60 },
  publishedAt: "2026-08-13T21:56:57Z",
  latest: true,
};

describe("an agent's ceilings", () => {
  it("says a ceiling nobody set in words", () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <AgentCapabilities agent={agent} />
      </QueryClientProvider>,
    );

    // Tokens has no ceiling here. It read "agents.noCeiling" on screen.
    expect(screen.getAllByText("sem teto").length).toBeGreaterThan(0);
    expect(screen.queryByText(/agents\./)).not.toBeInTheDocument();
  });
});
