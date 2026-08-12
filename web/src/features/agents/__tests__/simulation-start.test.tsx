import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SimulationStart } from "@/features/agents/simulation-start";

function renderStart(onStarted = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <SimulationStart agentId="triage" onStarted={onStarted} />
    </QueryClientProvider>,
  );
  return onStarted;
}

/** Answers the start with an accepted set, and records what was posted. */
function stubStart() {
  const posted: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      posted.push(
        input instanceof Request
          ? await input.text()
          : String(init?.body ?? ""),
      );
      return new Response(JSON.stringify({ id: "sim_triage_1", cases: 2 }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return posted;
}

describe("starting a simulation", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("will not start with nothing to run", async () => {
    // A set of zero cases is a report of zero rows somebody reads as a pass.
    renderStart();
    expect(screen.getByRole("button", { name: "Simular" })).toBeDisabled();
  });

  it("counts the cases as they are pasted, ignoring the trailing newline", async () => {
    const user = userEvent.setup();
    renderStart();

    await user.type(screen.getByLabelText("Casos"), '{{"a":1}\n{{"a":2}\n');

    expect(await screen.findByText("2 casos")).toBeInTheDocument();
  });

  it("hands the simulation back so the page can follow it", async () => {
    const posted = stubStart();
    const user = userEvent.setup();
    const onStarted = renderStart();

    await user.type(screen.getByLabelText("Casos"), '{{"a":1}');
    await user.click(screen.getByRole("button", { name: "Simular" }));

    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("sim_triage_1"));
    // The cases go up as they were written: the server splits and validates
    // them, and a second parse here could disagree about what a case is.
    expect(JSON.parse(posted[0]!)).toEqual({ cases: '{"a":1}' });
  });
});
