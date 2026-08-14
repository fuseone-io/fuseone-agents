import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { VersionComparison } from "@/features/agents/version-comparison";
import type { VersionComparison as Comparison } from "@/features/agents/simulation-api";

/*
Is this version better than the one before it?

The panel is absent whenever there is nothing to say. An empty table under a
heading reads as "nothing changed", which is a different answer from "these
two were never compared".
*/

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <VersionComparison agentId="triage" />
    </QueryClientProvider>,
  );
}

function stubComparison(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

const comparison: Comparison = {
  from: "v3f9a1c2b8d40",
  to: "v41b7e9d0c522",
  regressed: 1,
  fixed: 0,
  costMicros: 2_000,
  cases: [
    { id: "estorno", was: "held", now: "broke", costMicros: 2_000, steps: 5 },
    { id: "acesso", was: "held", now: "held", costMicros: 0, steps: 0 },
  ],
};

describe("comparing two versions", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("names the correction that stopped holding, not only the count", async () => {
    stubComparison(200, comparison);
    renderPanel();

    expect(await screen.findByText("estorno")).toBeInTheDocument();
    expect(screen.getByText(/1 correção deixou de valer/)).toBeInTheDocument();
  });

  it("says the money moved even when nothing broke", async () => {
    // The regression a held-and-broken count reports as no change at all.
    stubComparison(200, { ...comparison, regressed: 0, costMicros: 2_000 });
    renderPanel();

    expect(await screen.findByText(/a mais/)).toBeInTheDocument();
  });

  it("shows nothing when one version was never run against the corpus", async () => {
    stubComparison(409, { type: "fuseone:conflict", title: "Conflict", status: 409 });
    const { container } = renderPanel();

    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(container).toBeEmptyDOMElement();
  });
});
