import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApprovalPanel } from "@/features/runs/approval-panel";
import type { PendingApproval } from "@/lib/api/client";

const approval: PendingApproval = {
  runId: "run-1",
  tool: "crm.note",
  rule: "taint",
  reason: "arguments derive from untrusted data",
  requestedAt: "2026-08-11T09:00:00Z",
  atSeq: 10,
};

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ApprovalPanel runId="run-1" approval={approval} />
    </QueryClientProvider>,
  );
}

/** Captures the body of the decision request the panel sends. */
function stubDecisionEndpoint() {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      // openapi-fetch hands us a Request; read the body off whichever form.
      const body =
        input instanceof Request ? await input.clone().text() : String(init?.body);
      sent.push(JSON.parse(body));
      return new Response(JSON.stringify({ runId: "run-1", phase: "running" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  return sent;
}

describe("ApprovalPanel", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it("explains the rule in the reader's language, not the server's", () => {
    renderPanel();

    // `reason` is developer-facing English. The panel must render the
    // localised explanation derived from the stable `rule` key.
    expect(screen.getByText(/dado não confiável/i)).toBeInTheDocument();
    expect(screen.queryByText(approval.reason!)).not.toBeInTheDocument();
  });

  it("names the tool that will run so the approver knows what they are clearing", () => {
    renderPanel();
    expect(screen.getByText("crm.note")).toBeInTheDocument();
  });

  it("asks for confirmation before approving", async () => {
    const sent = stubDecisionEndpoint();
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Aprovar" }));

    // The first click opens the dialog; nothing has been decided yet.
    expect(sent).toHaveLength(0);
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  it("sends the step it is answering so a stale tab cannot decide the wrong action", async () => {
    const sent = stubDecisionEndpoint();
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Aprovar" }));
    const dialog = screen.getByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Aprovar" }));

    // Without atSeq the server would apply the decision to whatever the run is
    // waiting on now, which may be a different action entirely.
    await waitFor(() => expect(sent).toEqual([{ approved: true, atSeq: 10 }]));
  });

  it("rejects without a confirmation dialog, since refusing causes no effect", async () => {
    const sent = stubDecisionEndpoint();
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Recusar" }));

    await waitFor(() => expect(sent).toEqual([{ approved: false, atSeq: 10 }]));
  });
});
