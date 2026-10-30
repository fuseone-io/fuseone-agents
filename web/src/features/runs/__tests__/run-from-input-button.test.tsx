import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RunFromInputButton } from "@/features/runs/run-from-input-button";

function Landed() {
  const { runId } = useParams();
  return <p>chegou em {runId}</p>;
}

function renderButton() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/runs/run-1"]}>
        <Routes>
          <Route
            path="/runs/run-1"
            element={<RunFromInputButton runId="run-1" />}
          />
          <Route path="/runs/:runId" element={<Landed />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

type Sent = { url: string; key: string | null };

/** Answers every POST with the given reply, and records what was sent. */
function stubPost(status: number, body: unknown) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL) => {
      const request = input as Request;
      sent.push({
        url: request.url,
        key: request.headers.get("Idempotency-Key"),
      });
      const type =
        status < 300 ? "application/json" : "application/problem+json";
      return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": type },
      });
    }),
  );
  return sent;
}

const OPENED = {
  runId: "run-2",
  agentId: "triage",
  versionId: "v2",
  phase: "unstarted",
  seq: 1,
  startedAt: "2026-10-30T12:00:00Z",
  sourceRunId: "run-1",
  scope: { company: "acme", area: "cx" },
  cost: { micros: 0 },
};

afterEach(() => vi.unstubAllGlobals());

describe("running again from a recorded input", () => {
  it("asks before opening anything", async () => {
    const sent = stubPost(201, OPENED);
    renderButton();

    await userEvent.click(
      screen.getByRole("button", { name: /executar com esta entrada/i }),
    );

    expect(await screen.findByRole("alertdialog")).toBeInTheDocument();
    expect(sent).toHaveLength(0);
  });

  it("opens a new run from this one and goes to it", async () => {
    const sent = stubPost(201, OPENED);
    renderButton();

    await userEvent.click(
      screen.getByRole("button", { name: /executar com esta entrada/i }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /abrir nova execução/i }),
    );

    expect(await screen.findByText("chegou em run-2")).toBeInTheDocument();
    expect(sent[0]?.url).toMatch(/\/runs\/run-1\/rerun$/);
    expect(sent[0]?.key).toMatch(/^rerun-/);
  });

  it("says the recorded input is gone rather than opening anything", async () => {
    stubPost(409, {
      type: "fuseone:source-input-erased",
      title: "Source input erased",
      status: 409,
      detail: "run-1",
    });
    renderButton();

    await userEvent.click(
      screen.getByRole("button", { name: /executar com esta entrada/i }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /abrir nova execução/i }),
    );

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/apagada/i),
    );
  });
});
