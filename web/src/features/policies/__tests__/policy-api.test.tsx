import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  useDeletePolicy,
  usePolicies,
} from "@/features/policies/api";
import type { Policy } from "@/lib/api/client";

afterEach(() => vi.unstubAllGlobals());

describe("removing a policy", () => {
  it("deletes the selected code and refreshes the active list", async () => {
    let items = [policy("POL-100"), policy("POL-200")];
    let deletedPath = "";
    let listReads = 0;
    vi.stubGlobal("fetch", async (request: Request) => {
      const url = new URL(request.url);
      if (request.method === "DELETE") {
        deletedPath = url.pathname;
        items = items.filter((item) => item.code !== "POL-200");
        return new Response(null, { status: 204 });
      }
      listReads += 1;
      return Response.json({ items, policyHash: "hash" });
    });
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(
      () => ({ policies: usePolicies(), remove: useDeletePolicy() }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.policies.data?.items).toHaveLength(2));
    await act(() => result.current.remove.mutateAsync("POL-200"));

    expect(deletedPath).toBe("/api/v1/policies/POL-200");
    await waitFor(() =>
      expect(result.current.policies.data?.items.map((item) => item.code)).toEqual([
        "POL-100",
      ]),
    );
    expect(listReads).toBe(2);
  });
});

function policy(code: string): Policy {
  return {
    code,
    name: code,
    effect: "deny",
    mode: "enforce",
    enabled: true,
  };
}
