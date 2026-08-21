import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { SimulationStart } from "@/features/agents/simulation-start";
import { setLocale } from "@/i18n";
import type { Agent } from "@/lib/api/client";

const RUNNING_AGENT: Agent = {
  agentId: "triage",
  versionId: "v1",
  scope: { company: "acme", area: "platform" },
  name: "Triage",
  provider: "anthropic",
  model: "claude-sonnet-5",
  tools: [],
  budget: { micros: 0, steps: 20 },
  publishedAt: "2026-08-20T12:00:00Z",
  latest: true,
  paused: false,
};

function renderStart({
  onStarted = vi.fn(),
  agent = RUNNING_AGENT,
}: {
  onStarted?: (simulationId: string) => void;
  agent?: Agent;
} = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SimulationStart agentId="triage" agent={agent} onStarted={onStarted} />
      </MemoryRouter>
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
  beforeEach(() => {
    setLocale("en-US");
    vi.restoreAllMocks();
  });
  afterEach(() => vi.unstubAllGlobals());

  it("will not start a pasted set with nothing to run", async () => {
    // A set of zero cases is a report of zero rows somebody reads as a pass.
    const user = userEvent.setup();
    renderStart();

    await user.click(screen.getByRole("button", { name: "Paste JSON" }));

    expect(
      screen.getByRole("button", { name: "Rehearse 0 situations" }),
    ).toBeDisabled();
  });

  it("will not start while the agent is stopped", async () => {
    renderStart({ agent: { ...RUNNING_AGENT, paused: true } });

    expect(screen.getByText("This agent is stopped")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Rehearse saved situations" }),
    ).toBeDisabled();
  });

  it("counts the cases as they are pasted, ignoring the trailing newline", async () => {
    const user = userEvent.setup();
    renderStart();

    await user.click(screen.getByRole("button", { name: "Paste JSON" }));
    await user.type(screen.getByLabelText("Cases"), '{{"a":1}\n{{"a":2}\n');

    expect(await screen.findByText(/2 lines read/)).toBeInTheDocument();
  });

  it("starts from saved situations by default", async () => {
    const posted = stubStart();
    const user = userEvent.setup();
    const onStarted = renderStart();

    await user.click(
      screen.getByRole("button", { name: "Rehearse saved situations" }),
    );

    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("sim_triage_1"));
    expect(JSON.parse(posted[0]!)).toEqual({ corpus: true });
  });

  it("hands pasted JSON back so the page can follow it", async () => {
    const posted = stubStart();
    const user = userEvent.setup();
    const onStarted = renderStart();

    await user.click(screen.getByRole("button", { name: "Paste JSON" }));
    await user.type(screen.getByLabelText("Cases"), '{{"a":1}');
    await user.click(
      screen.getByRole("button", { name: "Rehearse 1 situation" }),
    );

    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("sim_triage_1"));
    // The cases go up as they were written: the server splits and validates
    // them, and a second parse here could disagree about what a case is.
    expect(JSON.parse(posted[0]!)).toEqual({ cases: '{"a":1}' });
  });

  it("turns a hand-written situation into JSONL only when rehearsing", async () => {
    const posted = stubStart();
    const user = userEvent.setup();
    const onStarted = renderStart();

    await user.click(screen.getByRole("button", { name: "Write my own" }));
    await user.type(screen.getByLabelText(/What arrived/), "Checkout alert");
    await user.type(
      screen.getByLabelText("The message itself"),
      "pod restarted twice",
    );
    await user.click(
      screen.getByRole("button", { name: "Add this situation" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Rehearse 1 situation" }),
    );

    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("sim_triage_1"));
    expect(JSON.parse(JSON.parse(posted[0]!).cases)).toEqual({
      subject: "Checkout alert",
      message: "pod restarted twice",
    });
  });
});
