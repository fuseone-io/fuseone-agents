import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useInstructionTokens } from "@/features/agents/instruction-tokens-api";

// The size of an instruction is asked of the provider a run of the agent would
// reach (NT-019): the agent's scope travels with the question.

afterEach(() => vi.unstubAllGlobals());

function asked() {
  const bodies: unknown[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    bodies.push(await request.json());
    return new Response(JSON.stringify({ model: "m", counted: true, tokens: 9 }), {
      headers: { "Content-Type": "application/json" },
    });
  });
  return bodies;
}

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("counting an instruction", () => {
  it("asks at the agent's scope", async () => {
    const bodies = asked();
    const { result } = renderHook(() => useInstructionTokens("cx-claude", "m", "Answer refunds.", "acme/cx"), { wrapper });

    await waitFor(() => expect(result.current).toBe(9), { timeout: 3000 });
    expect(bodies).toEqual([{ provider: "cx-claude", model: "m", instructions: "Answer refunds.", scope: "acme/cx" }]);
  });

  it("names no scope for an agent that has none yet", async () => {
    const bodies = asked();
    const { result } = renderHook(() => useInstructionTokens("shared", "m", "Answer refunds.", undefined), { wrapper });

    await waitFor(() => expect(result.current).toBe(9), { timeout: 3000 });
    expect(bodies).toEqual([{ provider: "shared", model: "m", instructions: "Answer refunds." }]);
  });
});
