import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AbandonDialog } from "@/features/runs/abandon-dialog";

const PLAN = {
  acts: [
    { tool: "crm.charge", seq: 8, undo: "crm.charge.refund" },
    { tool: "crm.email", seq: 4 },
  ],
};

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <AbandonDialog runId="run-1" />
    </QueryClientProvider>,
  );
}

/** Answers the plan read, and captures what gets posted. */
function stubEndpoints() {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL, init?: RequestInit) => {
      const method =
        input instanceof Request ? input.method : (init?.method ?? "GET");
      if (method === "GET") {
        return json(PLAN);
      }
      const body =
        input instanceof Request
          ? await input.clone().text()
          : String(init?.body);
      sent.push(JSON.parse(body));
      return json({ outcomes: [] });
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

afterEach(() => vi.unstubAllGlobals());

describe("ending a run", () => {
  it("shows what would be undone before anything can be pressed", async () => {
    stubEndpoints();
    renderDialog();

    await userEvent.click(screen.getByRole("button", { name: /encerrar/i }));

    // The act that has an undo, and the one that does not. The second is the
    // reason to show this at all: a sent email stays sent.
    expect(await screen.findByText("crm.charge.refund")).toBeInTheDocument();
    expect(screen.getByText(/nada desfaz/i)).toBeInTheDocument();
  });

  it("refuses to end a run without saying why", async () => {
    stubEndpoints();
    renderDialog();

    await userEvent.click(screen.getByRole("button", { name: /encerrar/i }));
    const confirm = await screen.findByRole("button", {
      name: /encerrar execução/i,
    });

    expect(confirm).toBeDisabled();
  });

  it("sends the reason and the decision to undo", async () => {
    const sent = stubEndpoints();
    renderDialog();

    await userEvent.click(screen.getByRole("button", { name: /encerrar/i }));
    await userEvent.type(
      await screen.findByLabelText(/por quê/i),
      "cobrança duplicada",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /encerrar execução/i }),
    );

    await waitFor(() =>
      expect(sent).toEqual([
        { reason: "cobrança duplicada", compensate: true },
      ]),
    );
  });

  it("records leaving the world as it is as a deliberate choice", async () => {
    const sent = stubEndpoints();
    renderDialog();

    await userEvent.click(screen.getByRole("button", { name: /encerrar/i }));
    await userEvent.click(await screen.findByLabelText(/desfazer o que ficou/i));
    await userEvent.type(await screen.findByLabelText(/por quê/i), "fica");
    await userEvent.click(
      screen.getByRole("button", { name: /encerrar execução/i }),
    );

    await waitFor(() =>
      expect(sent).toEqual([{ reason: "fica", compensate: false }]),
    );
  });
});
