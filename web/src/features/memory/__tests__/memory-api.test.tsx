import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useAcceptMemorySuggestion } from "@/features/memory/api";

afterEach(() => vi.unstubAllGlobals());

describe("accepting a memory suggestion", () => {
  it("sends a corrected claim only when the reviewer rewrote it", async () => {
    const bodies: unknown[] = [];
    vi.stubGlobal("fetch", async (request: Request) => {
      bodies.push(await request.clone().json());
      return new Response("{}", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useAcceptMemorySuggestion(), {
      wrapper,
    });

    await act(() =>
      result.current.mutateAsync({
        id: "suggestion_1",
        company: "acme",
        area: "ops",
        reason: "the proposal is accurate",
      }),
    );
    await act(() =>
      result.current.mutateAsync({
        id: "suggestion_2",
        company: "acme",
        area: "ops",
        reason: "clearer wording",
        claim: "the refund ceiling is R$ 500",
      }),
    );

    expect(bodies).toEqual([
      {
        company: "acme",
        area: "ops",
        reason: "the proposal is accurate",
      },
      {
        company: "acme",
        area: "ops",
        reason: "clearer wording",
        claim: "the refund ceiling is R$ 500",
      },
    ]);
  });
});
