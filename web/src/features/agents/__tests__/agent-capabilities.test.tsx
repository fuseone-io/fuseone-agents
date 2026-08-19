import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentCapabilities } from "@/features/agents/agent-capabilities";
import { setLocale } from "@/i18n";
import type { Agent } from "@/lib/api/client";

const admin = vi.hoisted(() => ({
  tools: [] as Array<{
    toolId: string;
    effect: "unknown" | "read" | "write" | "destructive" | "financial";
  }>,
}));

vi.mock("@/features/admin/api", () => ({
  useTools: () => ({ data: { items: admin.tools } }),
}));

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
  beforeEach(() => {
    admin.tools = [];
    setLocale("pt-BR");
  });

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

  it("renders tool effects in the current interface language", () => {
    admin.tools = [{ toolId: "crm.lookup", effect: "write" }];
    setLocale("en-US");
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    render(
      <QueryClientProvider client={client}>
        <AgentCapabilities agent={agent} />
      </QueryClientProvider>,
    );

    expect(screen.getByText("write")).toBeInTheDocument();
    expect(screen.queryByText("escrita")).not.toBeInTheDocument();
  });
});
