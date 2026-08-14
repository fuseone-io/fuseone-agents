import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/sonner";
import { PauseControl } from "@/features/agents/pause-control";

/*
Starting an agent from the console.

An agent is published stopped, so this is the last step of authoring one and
the first place a refusal is read: the server will not start an agent whose
corrections stopped holding, and the reason has to reach the person who just
pressed the switch.
*/

function renderControl(paused: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <PauseControl agentId="triage" paused={paused} />
      <Toaster />
    </QueryClientProvider>,
  );
}

function stubStart(status: number, problem?: unknown) {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    // The client hands fetch a whole Request, so what was asked for is on
    // the request rather than in an init object.
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

describe("starting and stopping an agent", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("says which state it is in, in words and not only in the switch", () => {
    renderControl(true);
    expect(screen.getByLabelText("Parado")).toBeInTheDocument();
  });

  it("starts it, which is asking for it to stop being paused", async () => {
    const sent = stubStart(204);
    renderControl(true);

    await userEvent.click(screen.getByRole("switch"));

    await waitFor(() => expect(sent).toEqual([{ paused: false }]));
  });

  it("shows the refusal when the corrections stopped holding", async () => {
    // The server names the cases. A toast that said only "failed" would send
    // somebody to read twenty runs to find out which one.
    stubStart(409, {
      type: "fuseone:conflict",
      title: "Conflict",
      status: 409,
      detail: "triage: 1 correction(s) no longer hold — caso-estorno.",
    });
    renderControl(true);

    await userEvent.click(screen.getByRole("switch"));

    expect(await screen.findByText(/caso-estorno/)).toBeInTheDocument();
  });
});
