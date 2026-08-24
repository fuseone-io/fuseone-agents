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

const PRICED_AGENT: Agent = {
  ...RUNNING_AGENT,
  budget: { micros: 500_000, steps: 20, tokens: 100_000 },
  activity: {
    runs: 5,
    finished: 5,
    waiting: 0,
    costMicros: 1_000_000,
  },
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

/** Answers the API calls this screen makes, and records simulation starts. */
function stubApi({
  regressions = [],
}: {
  regressions?: { id: string }[];
} = {}) {
  const posted: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : new Request(input, init);
      const path = new URL(request.url).pathname;
      if (request.method === "GET" && path.endsWith("/agents/triage/regressions")) {
        return new Response(JSON.stringify({ items: regressions }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      posted.push(await request.text());
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
    stubApi();
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
    const posted = stubApi();
    const user = userEvent.setup();
    const onStarted = renderStart();

    await user.click(
      screen.getByRole("button", { name: "Rehearse saved situations" }),
    );

    await waitFor(() => expect(onStarted).toHaveBeenCalledWith("sim_triage_1"));
    expect(JSON.parse(posted[0]!)).toEqual({ corpus: true });
  });

  it("hands pasted JSON back so the page can follow it", async () => {
    const posted = stubApi();
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

  it("shows the maximum money exposure before opening pasted cases", async () => {
    const user = userEvent.setup();
    renderStart({ agent: PRICED_AGENT });

    await user.click(screen.getByRole("button", { name: "Paste JSON" }));
    await user.type(screen.getByLabelText("Cases"), '{{"a":1}\n{{"a":2}\n');

    expect(await screen.findByText(/Expected about R\$0\.40/)).toBeInTheDocument();
    expect(screen.getByText(/maximum R\$1\.00/)).toBeInTheDocument();
  });

  it("counts the saved corpus before naming its maximum exposure", async () => {
    stubApi({ regressions: [{ id: "a" }, { id: "b" }, { id: "c" }] });

    renderStart({ agent: PRICED_AGENT });

    expect(await screen.findByText(/Expected about R\$0\.60/)).toBeInTheDocument();
    expect(screen.getByText(/maximum R\$1\.50/)).toBeInTheDocument();
  });

  it("does not pretend to know money exposure when the agent has no money ceiling", async () => {
    renderStart({ agent: RUNNING_AGENT });

    expect(
      await screen.findByText(
        "This agent has no money ceiling per run, so the maximum money exposure cannot be known before starting.",
      ),
    ).toBeInTheDocument();
  });

  it("turns a hand-written situation into JSONL only when rehearsing", async () => {
    const posted = stubApi();
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
