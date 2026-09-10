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
    memoryLearning: { mode: "review", ttlDays: 45 },
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
      memoryLearning: { mode: "review", ttlDays: 45 },
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

  it("names memory learning as a published change", () => {
    const { result, rerender } = renderHook(
      ({ loaded }) => useAgentDraft(loaded),
      {
        initialProps: { loaded: undefined as AgentDetail | undefined },
      },
    );

    rerender({ loaded: cobranca });
    act(() =>
      result.current.patch({
        memoryLearning: {
          mode: "auto_confirm",
          minObservations: 4,
          ttlDays: 30,
        },
      }),
    );

    expect(result.current.changes).toContainEqual(
      expect.objectContaining({ field: "agents.fieldMemoryLearning" }),
    );
  });
});

/*
 * The draft carries what no field on this screen shows.
 *
 * Publishing writes the draft whole, so a value the draft drops is a value
 * deleted on the next edit — silently, on a change somebody made for another
 * reason. The approval policy has a control of its own on the governance tab,
 * and still has to survive an edit made on a tab beside it without anybody
 * opening that one.
 */
describe("a version's approval policy", () => {
  it("survives an edit made for another reason", () => {
    const asking = {
      ...cobranca,
      approvals: { direct: true, notify: ["usr_ana"] },
    } as unknown as AgentDetail;

    const { result } = renderHook(() => useAgentDraft(asking));

    expect(result.current.draft.approvals).toEqual({
      direct: true,
      notify: ["usr_ana"],
    });

    act(() => result.current.patch({ model: "outro-modelo" }));

    expect(result.current.draft.approvals).toEqual({
      direct: true,
      notify: ["usr_ana"],
    });
  });

  it("is absent for an agent that asked for nothing", () => {
    const { result } = renderHook(() => useAgentDraft(cobranca));

    expect(result.current.draft.approvals).toBeUndefined();
  });
});
