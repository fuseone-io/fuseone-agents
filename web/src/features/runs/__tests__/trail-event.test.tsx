import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TrailEvent } from "@/features/runs/trail-event";
import type { Step } from "@/lib/api/client";

function renderEvent(step: Step) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ul>
        <TrailEvent
          runId="run-1"
          step={step}
          live={false}
          last
          showHashes={false}
        />
      </ul>
    </QueryClientProvider>,
  );
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("the run trail", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("opens a finished run's stored answer from the content endpoint", async () => {
    const answer = "Refunded R$ 88,21 to Maria Silva.";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | RequestInfo | URL) => {
        const url = input instanceof Request ? input.url : String(input);
        expect(url).toContain("/runs/run-1/steps/4/content");
        return json({ seq: 4, digest: "sha256:answer", content: answer });
      }),
    );

    renderEvent({
      seq: 4,
      kind: "run_finished",
      at: "2026-08-18T12:00:00Z",
      hash: "h",
      payload: {
        outcome_ref: "run://run-1/4/abc",
        outcome_digest: "sha256:answer",
      },
    });

    await userEvent.click(
      screen.getByRole("button", { name: /resposta final/i }),
    );

    expect(await screen.findByText(answer)).toBeInTheDocument();
  });
});
