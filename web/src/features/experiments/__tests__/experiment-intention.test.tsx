import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useStartExperiment } from "@/features/experiments/experiments-api";

/*
The key names one intention (NT-015): the server answers a retry of the same body with the
experiment it opened, and refuses another body under the same key. So a key lives as long as
its body has not reached an experiment, a changed body gets its own, and a body that comes
back gets its own back — that is the key its runs, if the answer was lost, are under.
*/

function stubAnswers(statuses: number[]) {
  const keys: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : new Request(input, init);
      keys.push(request.headers.get("Idempotency-Key") ?? "");
      const status = statuses.shift() ?? 202;
      const body = status === 202 ? { id: "exp_1", cases: 1, repetitions: 1, runs: 1, datasetVersion: 1 } : { title: "lost" };
      return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
    }),
  );
  return keys;
}

function startHook() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useStartExperiment("triage"), { wrapper }).result;
}

const once = { version: "v2", repetitions: 1 };
const twice = { version: "v2", repetitions: 2 };

describe("the key an experiment is started under", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("is kept for the same body, renewed for another, and given back when the body returns", async () => {
    const keys = stubAnswers([500, 500, 500]);
    const start = startHook();

    for (const body of [once, once, twice, once]) {
      await start.current.mutateAsync(body).catch(() => undefined);
    }

    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
    expect(keys[3]).toBe(keys[0]);
  });

  it("is retired once its experiment exists", async () => {
    const keys = stubAnswers([202, 202]);
    const start = startHook();

    await start.current.mutateAsync(once);
    await start.current.mutateAsync(once);

    expect(keys[1]).not.toBe(keys[0]);
  });
});
