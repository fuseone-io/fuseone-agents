import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { AgentPrimary } from "@/features/agents/agent-primary";
import type { Agent } from "@/lib/api/client";

/*
The one filled button in the bar.

Its position never moves and its verb follows the state, which is the whole
reason the row was rebuilt: six controls of the same weight left nobody able
to tell what the screen wanted them to do.
*/

const agent = (over: Partial<Agent> = {}): Agent => ({
  agentId: "triage",
  versionId: "v3f9a1c2b8d40",
  scope: { company: "acme", area: "cx" },
  name: "Triagem",
  provider: "openai",
  model: "gpt-test",
  tools: [],
  budget: {},
  publishedAt: "2026-08-11T12:00:00Z",
  latest: true,
  ...over,
});

function renderPrimary(over: Partial<Agent> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AgentPrimary agent={agent(over)} />
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function stub(status: number, problem?: unknown) {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      sent.push(await input.json());
      return new Response(problem ? JSON.stringify(problem) : null, {
        status,
        headers: { "Content-Type": "application/problem+json" },
      });
    }),
  );
  return sent;
}

describe("the agent's primary action", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("asks a stopped agent to start", async () => {
    const sent = stub(204);
    renderPrimary({ paused: true });

    await userEvent.click(screen.getByRole("button", { name: "Iniciar" }));

    await waitFor(() => expect(sent).toEqual([{ paused: false }]));
  });

  it("shows the corrections that stopped holding when starting is refused", async () => {
    // The gate's own sentence names the cases. "Could not start" alone would
    // send somebody to read twenty runs to find out which.
    stub(409, {
      type: "fuseone:conflict",
      title: "Conflict",
      status: 409,
      detail: "triage: 1 correction(s) no longer hold — caso-estorno.",
    });
    renderPrimary({ paused: true });

    await userEvent.click(screen.getByRole("button", { name: "Iniciar" }));

    expect(await screen.findByText(/caso-estorno/)).toBeInTheDocument();
  });

  it("offers to bring back an agent taken out of circulation", () => {
    // Never a start: restoring and starting are two decisions, and the
    // server leaves it stopped on purpose.
    renderPrimary({ retired: true, paused: true });

    expect(screen.getByRole("button", { name: /Reativar/ })).toBeInTheDocument();
  });
});
