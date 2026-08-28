import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryCreatePanel } from "@/features/memory/memory-create-panel";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

const MATCH = {
  own: {
    id: "mem_1",
    scope: { company: "acme", area: "ops" },
    agentId: "triage",
    kind: "incident",
    subject: "grafana datasource",
    signature: "grafana.datasource.down",
    claim: "Refresh the datasource token.",
    evidence: [],
    observations: 2,
    confirmed: 1,
    labels: [],
    status: "active" as const,
    createdBy: "usr_ana",
    createdAt: "2026-08-25T12:00:00Z",
    updatedBy: "usr_ana",
    updatedAt: "2026-08-25T12:00:00Z",
  },
};

describe("global memory authoring", () => {
  beforeEach(() => {
    setLocale("en-US");
    useActiveScope.setState({ company: "acme", area: "ops" });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("waits for the run and duplicate check before enabling save", async () => {
    const run = deferred<Response>();
    const match = deferred<Response>();
    const requests: RequestRecord[] = [];
    stubNetwork(requests, { run: run.promise, match: match.promise });
    renderPanel();

    fillMemory();
    await waitFor(() => expect(requests).toEqual([{ kind: "run" }]));
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();

    await act(() => run.resolve(json(runRecord())));
    await waitFor(() => expect(requests[1]?.kind).toBe("match"));
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();

    await act(() => match.resolve(json(MATCH)));
    expect(await screen.findByText("This memory already exists")).toBeVisible();
    expect(screen.getByRole("button", { name: "Save memory" })).toBeEnabled();
  });

  it("uses the evidence run agent only for the agent namespace", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests);
    const user = userEvent.setup();
    renderPanel();

    fillMemory();
    await waitFor(() => expect(matches(requests)).toHaveLength(1));
    expect(matches(requests)[0]).toEqual(matchBody("agent", "triage"));

    await user.click(screen.getByRole("radio", { name: "Every agent in this scope" }));
    await waitFor(() => expect(matches(requests)).toHaveLength(2));
    expect(matches(requests)[1]).toEqual(matchBody("shared"));
  });

  it("keeps save blocked when the evidence run cannot be inspected", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests, {
      run: Promise.resolve(new Response("{}", { status: 404 })),
    });
    renderPanel();

    fillMemory();

    expect(await screen.findByText("The evidence run could not be inspected")).toBeVisible();
    expect(screen.getByRole("button", { name: "Try again" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();
  });

  it("keeps save available when only the duplicate check fails", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests, {
      match: Promise.resolve(new Response("{}", { status: 500 })),
    });
    renderPanel();

    fillMemory();

    expect(await screen.findByText("Existing memory could not be checked")).toBeVisible();
    expect(screen.getByText(
      "You can retry or save; the server will still merge matching memory and reject conflicts.",
    )).toBeVisible();
    expect(screen.getByRole("button", { name: "Try again" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Save memory" })).toBeEnabled();
  });

  it("does not offer retry when an agent-scoped memory cites an agentless run", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests, { run: Promise.resolve(json(runRecord(null))) });
    const user = userEvent.setup();
    renderPanel();

    fillMemory();

    expect(await screen.findByText("The evidence run has no agent")).toBeVisible();
    expect(screen.getByText(
      "Choose another run, or make this memory shared.",
    )).toBeVisible();
    expect(screen.queryByRole("button", { name: "Try again" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();

    await user.click(screen.getByRole("radio", { name: "Every agent in this scope" }));

    await waitFor(() => expect(matches(requests)).toHaveLength(1));
    expect(screen.getByRole("button", { name: "Save memory" })).toBeEnabled();
  });

});

type RequestRecord = { kind: "run" } | { kind: "match"; body: unknown };

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryCreatePanel framed={false} />
    </QueryClientProvider>,
  );
}

function stubNetwork(
  requests: RequestRecord[],
  replies: { run?: Promise<Response>; match?: Promise<Response> } = {},
) {
  vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
    const path = new URL(request.url).pathname;
    if (path.endsWith("/runs/run_1")) {
      requests.push({ kind: "run" });
      return replies.run ?? json(runRecord());
    }
    if (path.endsWith("/memory/match")) {
      requests.push({ kind: "match", body: await request.clone().json() });
      return replies.match ?? json({});
    }
    throw new Error(`unexpected request: ${path}`);
  }));
}

function fillMemory() {
  const fields = {
    Kind: "incident",
    Subject: "grafana datasource",
    Signature: "grafana.datasource.down",
    Claim: "Refresh the datasource token.",
    Run: "run_1",
    Why: "Reviewed after close",
  };
  for (const [label, value] of Object.entries(fields)) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  }
}

function matches(requests: RequestRecord[]) {
  return requests.flatMap((request) => request.kind === "match" ? [request.body] : []);
}

function matchBody(namespace: "agent" | "shared", agentId?: string) {
  return {
    company: "acme",
    area: "ops",
    namespace,
    ...(agentId ? { agentId } : {}),
    kind: "incident",
    subject: "grafana datasource",
    signature: "grafana.datasource.down",
  };
}

function runRecord(agentId: string | null = "triage") {
  return {
    runId: "run_1",
    ...(agentId ? { agentId } : {}),
    scope: { company: "acme", area: "ops" },
  };
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
