import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PendingDecision } from "@/features/runs/pending-decision";
import type { PendingApproval, Step } from "@/lib/api/client";

const approval: PendingApproval = {
  runId: "run-1",
  tool: "crm.note",
  rule: "taint",
  reason: "arguments derive from untrusted data",
  requestedAt: "2026-08-11T09:00:00Z",
  atSeq: 10,
};

const ARGS = '{"text":"reembolso do pedido #88213"}';

// Shaped like the ledger writes it: the taint the arguments carry is part of
// the request, and the effect is the domain's integer.
const WRITE = 2;

const step: Step = {
  seq: 10,
  kind: "approval_requested",
  at: "2026-08-11T09:00:00Z",
  hash: "abc",
  payload: {
    tool: "crm.note",
    effect: WRITE,
    labels: ["untrusted"],
    estimate: { micros: 4000 },
  },
};

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <PendingDecision runId="run-1" approval={approval} step={step} />
    </QueryClientProvider>,
  );
}

/** Answers the content read, and captures the decision that is posted. */
function stubEndpoints() {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/content")) {
        return json({ seq: 10, digest: "d1", content: ARGS });
      }
      const body =
        input instanceof Request
          ? await input.clone().text()
          : String(init?.body);
      sent.push(JSON.parse(body));
      return json({ runId: "run-1", phase: "running" });
    }),
  );
  return sent;
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("the pending decision", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("shows the arguments that will actually be sent", async () => {
    // The screen's whole reason to exist: never ask for an approval without
    // showing what will run.
    stubEndpoints();
    renderCard();

    expect(await screen.findByText(/88213/)).toBeInTheDocument();
  });

  it("says where the arguments came from, which is why a human was asked", async () => {
    stubEndpoints();
    renderCard();

    expect(await screen.findByText("untrusted")).toBeInTheDocument();
  });

  it("names the effect in words, never the number the ledger stores", async () => {
    stubEndpoints();
    renderCard();

    expect(await screen.findByText("write")).toBeInTheDocument();
    expect(screen.queryByText(String(WRITE))).not.toBeInTheDocument();
  });

  it("explains the rule in the reader's language, not the server's", () => {
    stubEndpoints();
    renderCard();

    expect(screen.getByText(/dado não confiável/i)).toBeInTheDocument();
    expect(screen.queryByText(approval.reason!)).not.toBeInTheDocument();
  });

  it("asks for confirmation before approving", async () => {
    const sent = stubEndpoints();
    const user = userEvent.setup();
    renderCard();

    await user.click(screen.getByRole("button", { name: "Aprovar" }));

    expect(sent).toHaveLength(0);
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  it("sends the step it is answering so a stale tab cannot decide the wrong action", async () => {
    const sent = stubEndpoints();
    const user = userEvent.setup();
    renderCard();

    await user.click(screen.getByRole("button", { name: "Aprovar" }));
    const dialog = screen.getByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Aprovar" }));

    await waitFor(() => expect(sent).toEqual([{ approved: true, atSeq: 10 }]));
  });

  it("refuses without a confirmation dialog, since refusing causes no effect", async () => {
    const sent = stubEndpoints();
    const user = userEvent.setup();
    renderCard();

    await user.click(screen.getByRole("button", { name: "Recusar" }));

    await waitFor(() => expect(sent).toEqual([{ approved: false, atSeq: 10 }]));
  });

  it("says the arguments are gone rather than showing an empty block", async () => {
    // Retention deletes the content while the step stays in the chain. Blank
    // arguments would read as "this call sends nothing".
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 404 })),
    );
    renderCard();

    expect(
      await screen.findByText(/não estão mais disponíveis/i),
    ).toBeInTheDocument();
  });
});
