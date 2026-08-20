import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useAgentDraft } from "@/features/agents/agent-draft";
import type { AgentDetail } from "@/lib/api/client";

const cobranca = {
  agent: {
    agentId: "cobranca",
    name: "Cobrança amigável",
    scope: { company: "default", area: "financeiro" },
    provider: "openai",
    model: "devstack",
    tools: ["crm.lookup"],
    budget: { micros: 500_000, steps: 60 },
  },
  instructions: "Cobre com educação.",
} as unknown as AgentDetail;

describe("the agent draft", () => {
  it("takes the agent's fields when it arrives after the first render", () => {
    // A cold load of /agents/{id}/edit renders before the query resolves. A
    // draft seeded only once shows a blank form for a real agent, and
    // publishing from it would replace the definition with empty fields.
    const { result, rerender } = renderHook(
      ({ loaded }) => useAgentDraft(loaded),
      {
        initialProps: { loaded: undefined as AgentDetail | undefined },
      },
    );

    expect(result.current.draft.name).toBe("");
    rerender({ loaded: cobranca });
    expect(result.current.draft).toMatchObject({
      name: "Cobrança amigável",
      company: "default",
      area: "financeiro",
      instructions: "Cobre com educação.",
    });
  });

  it("does not overwrite what somebody typed when the query refetches", () => {
    const { result, rerender } = renderHook(
      ({ loaded }) => useAgentDraft(loaded),
      {
        initialProps: { loaded: undefined as AgentDetail | undefined },
      },
    );

    rerender({ loaded: cobranca });
    act(() => result.current.patch({ name: "Cobrança firme" }));
    rerender({ loaded: cobranca });

    expect(result.current.draft.name).toBe("Cobrança firme");
  });
});
