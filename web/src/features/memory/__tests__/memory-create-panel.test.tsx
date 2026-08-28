import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
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
    const user = userEvent.setup();
    renderPanel();

    await fillMemory(user);
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

    await fillMemory(user);
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
    const user = userEvent.setup();
    renderPanel();

    await fillMemory(user);

    expect(await screen.findByText("Existing memory could not be checked")).toBeVisible();
    expect(screen.getByRole("button", { name: "Try again" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();
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

async function fillMemory(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Kind"), "incident");
  await user.type(screen.getByLabelText("Subject"), "grafana datasource");
  await user.type(screen.getByLabelText("Signature"), "grafana.datasource.down");
  await user.type(screen.getByLabelText("Claim"), "Refresh the datasource token.");
  await user.type(screen.getByLabelText("Run"), "run_1");
  await user.type(screen.getByLabelText("Why"), "Reviewed after close");
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

function runRecord() {
  return {
    runId: "run_1",
    agentId: "triage",
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
