import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryCreatePanel } from "@/features/memory/memory-create-panel";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";

const scopeHooks = vi.hoisted(() => ({
  items: [
    { company: "acme", area: "ops", label: "Operations" },
    { company: "globex", area: "security", label: "Security" },
  ],
}));

vi.mock("@/features/scope/api", () => ({
  useScopes: () => ({
    data: { items: scopeHooks.items },
    isLoading: false,
    isError: false,
  }),
}));

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

    await fillMemory();
    await waitFor(() => expect(details(requests)).toHaveLength(1));
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();

    await act(() => run.resolve(json(runRecord())));
    await waitFor(() => expect(matches(requests)).toHaveLength(1));
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

    await fillMemory();
    await waitFor(() => expect(matches(requests)).toHaveLength(1));
    expect(matches(requests)[0]).toEqual(matchBody("agent", "triage"));

    await user.click(screen.getByRole("radio", { name: "Every agent in this scope" }));
    await waitFor(() => expect(matches(requests)).toHaveLength(2));
    expect(matches(requests)[1]).toEqual(matchBody("shared"));
  });

  it("chooses an accessible company and area as one scope", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests, {
      run: Promise.resolve(
        json(runRecord("triage", { company: "globex", area: "security" })),
      ),
    });
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("combobox", { name: "Scope" }));
    expect(screen.getByPlaceholderText("Search company or area")).toBeVisible();
    await user.click(
      await screen.findByRole("option", { name: /security.*Security/i }),
    );
    expect(screen.getByRole("combobox", { name: "Scope" })).toHaveTextContent(
      "globex/security",
    );

    await fillMemory();
    await waitFor(() => expect(matches(requests)).toHaveLength(1));
    expect(matches(requests)[0]).toEqual({
      ...matchBody("agent", "triage"),
      company: "globex",
      area: "security",
    });
  });

  it("keeps save blocked when the evidence run cannot be inspected", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests, {
      run: Promise.resolve(new Response("{}", { status: 404 })),
    });
    renderPanel();

    await fillMemory();

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

    await fillMemory();

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

    await fillMemory();

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

  it("asks a global viewer to choose one accessible scope", async () => {
    useActiveScope.setState({ company: "*", area: "ops" });
    const user = userEvent.setup();
    renderPanel();

    expect(screen.getByRole("status")).toHaveTextContent(
      "Choose a company and area before saving. Memory cannot be recorded across every company.",
    );
    expect(screen.getByRole("combobox", { name: "Scope" })).toHaveTextContent(
      "Choose a company and area",
    );
    expect(screen.getByRole("button", { name: "Save memory" })).toBeDisabled();

    await user.click(screen.getByRole("combobox", { name: "Scope" }));
    await user.click(
      await screen.findByRole("option", { name: /ops.*Operations/i }),
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("resolves an exact finished run inside the chosen scope", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests);
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("combobox", { name: "Evidence run" }));
    await waitFor(() => expect(lists(requests)).toHaveLength(1));
    expect(lists(requests)[0]).toMatchObject({
      company: "acme",
      area: "ops",
      phase: "finished",
      limit: "25",
    });

    await user.type(
      screen.getByPlaceholderText("Search run ID or agent"),
      "run_ticketito_42",
    );
    await waitFor(() => expect(lists(requests)).toHaveLength(2));
    expect(lists(requests)[1]).toMatchObject({
      company: "acme",
      area: "ops",
      phase: "finished",
      q: "run_ticketito_42",
    });
    const option = await screen.findByRole("option", {
      name: /run_ticketito_42.*ticketito/i,
    });
    expect(within(option).getByText("Outside data")).toBeVisible();
    await user.click(option);
    expect(screen.getByRole("combobox", { name: "Evidence run" })).toHaveTextContent(
      "run_ticketito_42",
    );
  });

  it("says when more finished runs exist beyond the first page", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests, {
      list: () => json({
        items: Array.from({ length: 25 }, (_, index) =>
          runSummary(`run_${index + 1}`),
        ),
        nextCursor: "run_25",
      }),
    });
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByRole("combobox", { name: "Evidence run" }));

    expect(
      await screen.findByText(
        "More finished runs exist. Refine the search to find them.",
      ),
    ).toBeVisible();
  });

  it("clears a selected evidence run when the scope changes", async () => {
    const requests: RequestRecord[] = [];
    stubNetwork(requests);
    const user = userEvent.setup();
    renderPanel();

    await chooseEvidenceRun(user);
    expect(screen.getByRole("combobox", { name: "Evidence run" })).toHaveTextContent(
      "run_1",
    );

    await user.click(screen.getByRole("combobox", { name: "Scope" }));
    await user.click(
      await screen.findByRole("option", { name: /security.*Security/i }),
    );

    expect(screen.getByRole("combobox", { name: "Evidence run" })).toHaveTextContent(
      "Choose a finished run",
    );
  });

});

type RequestRecord =
  | { kind: "list"; query: Record<string, string> }
  | { kind: "run" }
  | { kind: "match"; body: unknown };

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
  replies: {
    list?: (url: URL) => Response | Promise<Response>;
    run?: Promise<Response>;
    match?: Promise<Response>;
  } = {},
) {
  vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
    const url = new URL(request.url);
    const path = url.pathname;
    if (path.endsWith("/runs")) {
      requests.push({ kind: "list", query: Object.fromEntries(url.searchParams) });
      if (replies.list) return replies.list(url);
      return json({
        items: url.searchParams.get("q")
          ? [runSummary("run_ticketito_42", "ticketito")]
          : [runSummary()],
      });
    }
    if (path.endsWith("/runs/run_1")) {
      requests.push({ kind: "run" });
      return replies.run ?? json(runRecord());
    }
    if (path.endsWith("/runs/run_ticketito_42")) {
      requests.push({ kind: "run" });
      return json(runRecord("ticketito", undefined, "run_ticketito_42"));
    }
    if (path.endsWith("/memory/match")) {
      requests.push({ kind: "match", body: await request.clone().json() });
      return replies.match ?? json({});
    }
    throw new Error(`unexpected request: ${path}`);
  }));
}

async function fillMemory() {
  const fields = {
    Kind: "incident",
    Subject: "grafana datasource",
    Signature: "grafana.datasource.down",
    Claim: "Refresh the datasource token.",
    Why: "Reviewed after close",
  };
  for (const [label, value] of Object.entries(fields)) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  }
  await chooseEvidenceRun(userEvent.setup());
}

async function chooseEvidenceRun(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("combobox", { name: "Evidence run" }));
  await user.click(await screen.findByRole("option", { name: /run_1/ }));
}

function matches(requests: RequestRecord[]) {
  return requests.flatMap((request) => request.kind === "match" ? [request.body] : []);
}

function details(requests: RequestRecord[]) {
  return requests.filter((request) => request.kind === "run");
}

function lists(requests: RequestRecord[]) {
  return requests.flatMap((request) => request.kind === "list" ? [request.query] : []);
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

function runRecord(
  agentId: string | null = "triage",
  scope = { company: "acme", area: "ops" },
  runId = "run_1",
) {
  return {
    runId,
    ...(agentId ? { agentId } : {}),
    scope,
  };
}

function runSummary(runId = "run_1", agentId = "triage") {
  return {
    ...runRecord(agentId, undefined, runId),
    versionId: "v1",
    phase: "finished",
    seq: 8,
    startedAt: "2026-08-28T12:00:00Z",
    endedAt: "2026-08-28T12:01:00Z",
    cost: { micros: 1000 },
    labels: ["untrusted"],
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
