import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStartRun } from "@/features/agents/start-run";

function Trigger() {
  const start = useStartRun("triage");
  return (
    <button onClick={() => start.mutate(undefined)} disabled={false}>
      Executar
    </button>
  );
}

function renderTrigger() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Trigger />
    </QueryClientProvider>,
  );
}

/** Captures the idempotency key of every attempt, and fails them all. */
function stubFailing() {
  const keys: (string | null)[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL) => {
      keys.push(input instanceof Request ? input.headers.get("Idempotency-Key") : null);
      return new Response("{}", { status: 500 });
    }),
  );
  return keys;
}

describe("opening a run", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("reuses one key across attempts at the same intention", async () => {
    // The key is unique per intention, not per attempt. A key minted per
    // request makes the header decorative: somebody clicking again after a
    // request that never answered opens a second run, and a run is real
    // tools against real systems.
    const keys = stubFailing();
    const user = userEvent.setup();
    renderTrigger();

    const button = screen.getByRole("button", { name: "Executar" });
    await user.click(button);
    await waitFor(() => expect(keys).toHaveLength(1));
    await user.click(button);
    await waitFor(() => expect(keys).toHaveLength(2));

    expect(keys[0]).toBe(keys[1]);
  });

  it("sends a key at all", async () => {
    const keys = stubFailing();
    const user = userEvent.setup();
    renderTrigger();

    await user.click(screen.getByRole("button", { name: "Executar" }));

    await waitFor(() => expect(keys[0]).toMatch(/^manual-/));
  });
});
