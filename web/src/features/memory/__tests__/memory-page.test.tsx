import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryPage } from "@/features/memory/memory-page";
import { useActiveScope } from "@/features/scope/active-scope";
import { setLocale } from "@/i18n";
import type { MemoryAssertion, MemorySuggestion } from "@/features/memory/api";

const hooks = vi.hoisted(() => ({
  items: [] as MemoryAssertion[],
  suggestions: [] as MemorySuggestion[],
  can: ["agent:read", "agent:publish"] as string[],
  create: vi.fn(),
  disable: vi.fn(),
  accept: vi.fn(),
  dismiss: vi.fn(),
  refetch: vi.fn(),
  suggestionsRefetch: vi.fn(),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@/features/session/api", () => ({
  useMe: () => ({ data: { can: hooks.can } }),
}));

vi.mock("@/features/memory/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/memory/api")>();
  return {
    ...actual,
    useMemoryAssertions: () => ({
      data: { items: hooks.items },
      isLoading: false,
      error: null,
      refetch: hooks.refetch,
    }),
    useMemorySuggestions: () => ({
      data: { items: hooks.suggestions },
      isLoading: false,
      error: null,
      refetch: hooks.suggestionsRefetch,
    }),
    useCreateMemoryAssertion: () => ({
      mutateAsync: hooks.create,
      isPending: false,
    }),
    useDisableMemoryAssertion: () => ({
      mutate: hooks.disable,
      isPending: false,
    }),
    useAcceptMemorySuggestion: () => ({
      mutate: hooks.accept,
      isPending: false,
    }),
    useDismissMemorySuggestion: () => ({
      mutate: hooks.dismiss,
      isPending: false,
    }),
  };
});

describe("governed memory page", () => {
  beforeEach(() => {
    setLocale("en-US");
    hooks.items = [];
    hooks.suggestions = [];
    hooks.can = ["agent:read", "agent:publish"];
    hooks.create.mockReset().mockResolvedValue(memoryAssertion(0));
    hooks.disable.mockReset();
    hooks.accept.mockReset();
    hooks.dismiss.mockReset();
    useActiveScope.setState({ company: "acme", area: "ops" });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("shows that more remembered assertions exist instead of cutting silently", () => {
    hooks.items = Array.from({ length: 9 }, (_, index) =>
      memoryAssertion(index),
    );
    render(<MemoryPage />);

    for (const name of [
      "Active",
      "Disabled",
      "Suggested memory",
      "All states",
    ]) {
      expect(
        screen.getByRole("tab", { name }).querySelector("svg"),
      ).not.toBeNull();
    }
    expect(screen.getAllByText("subject-0").length).toBeGreaterThan(0);
    expect(screen.getByText("subject-7")).toBeInTheDocument();
    expect(screen.queryByText("subject-8")).not.toBeInTheDocument();
    expect(screen.getByText("8 of 9")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Load more" }),
    ).toBeInTheDocument();
  });

  it("explains when a manual search exceeds the term budget", async () => {
    const user = userEvent.setup();
    render(<MemoryPage />);

    expect(
      screen.queryByText(
        "Only the first 6 search terms are used. Try stronger identifiers if nothing matches.",
      ),
    ).not.toBeInTheDocument();

    await user.type(
      screen.getByRole("searchbox", { name: "Search memory" }),
      "alertas do superset entregues no slack com erro not_in_channel hoje",
    );

    expect(screen.getByRole("status")).toHaveTextContent(
      "Only the first 6 search terms are used. Try stronger identifiers if nothing matches.",
    );
  });

  it("keeps remembered assertions compact until one is selected", async () => {
    const user = userEvent.setup();
    hooks.items = Array.from({ length: 3 }, (_, index) => ({
      ...memoryAssertion(index),
      claim: `Claim ${index}`,
      evidence: [
        {
          runId: `run_${index}`,
          artifact: "final_answer",
          digest: `sha256:${index}`,
        },
      ],
    }));
    render(<MemoryPage />);

    const third = screen.getByRole("button", { name: /subject-2/ });
    expect(within(third).getByText("Outside data")).toBeInTheDocument();
    expect(
      screen.getByText("run_0 · final_answer · sha256:0"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("run_2 · final_answer · sha256:2"),
    ).not.toBeInTheDocument();

    await user.click(third);

    expect(
      screen.getByText("run_2 · final_answer · sha256:2"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("run_0 · final_answer · sha256:0"),
    ).not.toBeInTheDocument();
  });

  it("leaves creation when a remembered assertion is selected", async () => {
    const user = userEvent.setup();
    hooks.items = [memoryAssertion(0), memoryAssertion(1)];
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await user.click(screen.getByRole("button", { name: /subject-1/ }));

    expect(screen.queryByText("New memory")).not.toBeInTheDocument();
    expect(screen.getByText("run_1 · final_answer · sha256:abcd")).toBeInTheDocument();
  });

  it("leaves creation when a pending suggestion is selected", async () => {
    const user = userEvent.setup();
    hooks.suggestions = [memorySuggestion(0), memorySuggestion(1)];
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await user.click(screen.getByRole("button", { name: /suggested-subject-1/ }));

    expect(screen.queryByText("New memory")).not.toBeInTheDocument();
    expect(screen.getByText("run_s_1 · memory_suggestion · sha256:bcde")).toBeInTheDocument();
  });

  it("discards an untouched creation panel when it is closed", async () => {
    const user = userEvent.setup();
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("button", { name: "Record memory" }));
    expect(screen.queryByRole("button", { name: "Discard draft" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(screen.queryByText("New memory")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Record memory" })).toBeVisible();
  });

  it("discards a dirty draft only after confirmation", async () => {
    const user = userEvent.setup();
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await user.type(screen.getByLabelText("Subject"), "grafana datasource");
    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByLabelText("Subject")).toHaveValue("grafana datasource");

    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    expect(screen.queryByText("New memory")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Record memory" }));
    expect(screen.getByLabelText("Subject")).toHaveValue("");
  });

  it("keeps the suggested view after discarding a draft", async () => {
    const user = userEvent.setup();
    hooks.suggestions = [memorySuggestion(0)];
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await user.type(screen.getByLabelText("Subject"), "grafana datasource");
    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    const dialog = screen.getByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Discard draft" }));

    expect(screen.getByRole("tab", { name: "Suggested memory" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("run_s_0 · memory_suggestion · sha256:bcde")).toBeVisible();
  });

  it("returns to memory without discarding the creation draft", async () => {
    const user = userEvent.setup();
    hooks.items = [memoryAssertion(0)];
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await user.type(screen.getByLabelText("Subject"), "grafana datasource");
    await user.click(screen.getByRole("button", { name: "Close" }));
    await user.click(screen.getByRole("button", { name: "Continue draft" }));

    expect(screen.getByLabelText("Subject")).toHaveValue("grafana datasource");
  });

  it("moves a preserved draft to the newly active scope", async () => {
    const user = userEvent.setup();
    hooks.items = [memoryAssertion(0)];
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await user.type(screen.getByLabelText("Subject"), "grafana datasource");
    await user.click(screen.getByRole("button", { name: "Close" }));
    act(() => useActiveScope.setState({ company: "globex", area: "security" }));
    await user.click(screen.getByRole("button", { name: "Continue draft" }));

    expect(screen.getByLabelText("Company")).toHaveValue("globex");
    expect(screen.getByLabelText("Area")).toHaveValue("security");
    expect(screen.getByLabelText("Subject")).toHaveValue("grafana datasource");
  });

  it("does not inspect a creation draft while its panel is hidden", async () => {
    const user = userEvent.setup();
    const fetch = vi.fn(async () => json({}));
    vi.stubGlobal("fetch", fetch);
    hooks.items = [memoryAssertion(0)];
    renderMemoryPageWithClient();

    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await fillMemoryIdentity(user);
    await user.type(screen.getByLabelText("Run"), "run_1");
    await user.click(screen.getByRole("button", { name: "Close" }));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
    });

    expect(fetch).not.toHaveBeenCalled();
  });

  it("records a reviewed assertion with ledger evidence", async () => {
    stubMemoryAuthoringNetwork();
    const user = userEvent.setup();
    renderMemoryPageWithClient();
    await user.click(screen.getByRole("button", { name: "Record memory" }));
    await fillAssertion(user);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Save memory" })).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: "Save memory" }));

    // No agent, no counters, no expiry: the server reads the agent off the run
    // the evidence names and owns the rest. The form asks only what a person
    // can answer.
    expect(hooks.create).toHaveBeenCalledWith({
      company: "acme",
      area: "ops",
      namespace: "agent",
      kind: "incident",
      subject: "grafana datasource",
      signature: "grafana.datasource.down",
      claim: "Refresh the datasource token before restarting the worker.",
      reason: "Reviewed after close",
      // The run, and nothing else: the artifact defaults to the closing answer
      // and the digest is the ledger's to say.
      evidence: [{ runId: "run_1" }],
    });
  });

  it("corrects a remembered assertion without dropping its evidence or its namespace", async () => {
    const user = userEvent.setup();
    hooks.items = [
      {
        ...memoryAssertion(0),
        evidence: [
          { runId: "run_1", artifact: "final_answer", digest: "sha256:abcd" },
          {
            runId: "run_2",
            artifact: "memory_suggestion",
            digest: "sha256:bcde",
          },
        ],
        expiresAt: "2026-09-25T12:00:00Z",
      },
    ];
    render(<MemoryPage />);

    await user.click(screen.getByRole("button", { name: "Correct" }));
    const dialog = screen.getByRole("alertdialog");
    expect(
      within(dialog).getByRole("button", { name: "Correct" }),
    ).toBeDisabled();
    await user.clear(within(dialog).getByLabelText("Claim"));
    await user.type(
      within(dialog).getByLabelText("Claim"),
      "Refresh the datasource token, then verify the datasource health endpoint.",
    );
    await user.type(within(dialog).getByLabelText("Why"), "runbook narrowed");
    await user.click(within(dialog).getByRole("button", { name: "Correct" }));

    expect(hooks.create).toHaveBeenCalledWith({
      company: "acme",
      area: "ops",
      namespace: "agent",
      kind: "incident",
      subject: "subject-0",
      signature: "signature-0",
      claim:
        "Refresh the datasource token, then verify the datasource health endpoint.",
      reason: "runbook narrowed",
      evidence: [
        { runId: "run_1", artifact: "final_answer", digest: "sha256:abcd" },
        {
          runId: "run_2",
          artifact: "memory_suggestion",
          digest: "sha256:bcde",
        },
      ],
    });
  });

  it("requires a reason before disabling a remembered assertion", async () => {
    const user = userEvent.setup();
    hooks.items = [memoryAssertion(0)];
    render(<MemoryPage />);

    await user.click(screen.getByRole("button", { name: "Disable" }));
    const dialog = screen.getByRole("alertdialog");
    expect(
      within(dialog).getByRole("button", { name: "Disable" }),
    ).toBeDisabled();
    await user.type(within(dialog).getByLabelText("Why"), "superseded");
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));

    expect(hooks.disable.mock.calls[0]?.[0]).toEqual({
      id: "mem_0",
      company: "acme",
      area: "ops",
      reason: "superseded",
    });
  });

  it("reviews a pending suggestion inside its own scope", async () => {
    const user = userEvent.setup();
    hooks.suggestions = [memorySuggestion(0)];
    render(<MemoryPage />);

    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    await user.click(screen.getByRole("button", { name: "Accept suggestion" }));
    const dialog = screen.getByRole("alertdialog");
    expect(
      within(dialog).getByRole("button", { name: "Accept suggestion" }),
    ).toBeDisabled();
    await user.type(within(dialog).getByLabelText("Why"), "observed twice");
    await user.click(
      within(dialog).getByRole("button", { name: "Accept suggestion" }),
    );

    // No claim: they agreed with the wording as well as with the fact, and
    // sending back the text they did not touch would record a different
    // agreement from the one they made.
    expect(hooks.accept.mock.calls[0]?.[0]).toEqual({
      id: "suggestion_0",
      company: "acme",
      area: "ops",
      reason: "observed twice",
    });
  });

  it("accepts a suggestion in better words", async () => {
    const user = userEvent.setup();
    hooks.suggestions = [memorySuggestion(0)];
    render(<MemoryPage />);

    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    await user.click(screen.getByRole("button", { name: "Accept suggestion" }));
    const dialog = screen.getByRole("alertdialog");
    const claim = within(dialog).getByLabelText("Claim");
    await user.clear(claim);
    await user.type(claim, "the refund ceiling is R$ 500");
    await user.type(within(dialog).getByLabelText("Why"), "clearer wording");
    await user.click(
      within(dialog).getByRole("button", { name: "Accept suggestion" }),
    );

    expect(hooks.accept.mock.calls[0]?.[0]).toMatchObject({
      claim: "the refund ceiling is R$ 500",
      reason: "clearer wording",
    });
  });

  // The identity is what runs search for. Rewriting it here would mark this
  // proposal accepted for a fact nobody proposed, so it is not offered.
  it("does not offer to rewrite the identity of a proposal", async () => {
    const user = userEvent.setup();
    hooks.suggestions = [memorySuggestion(0)];
    render(<MemoryPage />);

    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    await user.click(screen.getByRole("button", { name: "Accept suggestion" }));
    const dialog = screen.getByRole("alertdialog");

    expect(within(dialog).queryByLabelText("Subject")).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText("Signature")).not.toBeInTheDocument();
  });

  it("shows that more memory suggestions exist instead of cutting silently", async () => {
    const user = userEvent.setup();
    hooks.suggestions = Array.from({ length: 9 }, (_, index) =>
      memorySuggestion(index),
    );

    render(<MemoryPage />);

    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    expect(screen.getAllByText("suggested-subject-0").length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText("suggested-subject-7")).toBeInTheDocument();
    expect(screen.queryByText("suggested-subject-8")).not.toBeInTheDocument();
    expect(screen.getByText("8 of 9")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Load more" }),
    ).toBeInTheDocument();
  });

  it("keeps write controls out for read-only callers", async () => {
    const user = userEvent.setup();
    hooks.can = ["agent:read"];
    hooks.items = [memoryAssertion(0)];
    hooks.suggestions = [memorySuggestion(0)];

    render(<MemoryPage />);

    expect(screen.queryByRole("button", { name: "Record memory" })).toBeNull();
    expect(screen.queryByText("New memory")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Correct" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Disable" })).toBeNull();
    await user.click(screen.getByRole("tab", { name: "Suggested memory" }));
    expect(
      screen.queryByRole("button", { name: "Accept suggestion" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Dismiss suggestion" }),
    ).toBeNull();
  });
});

async function fillAssertion(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Kind"), "incident");
  await user.type(screen.getByLabelText("Subject"), "grafana datasource");
  await user.type(
    screen.getByLabelText("Signature"),
    "grafana.datasource.down",
  );
  await user.type(
    screen.getByLabelText("Claim"),
    "Refresh the datasource token before restarting the worker.",
  );
  await user.type(screen.getByLabelText("Run"), "run_1");
  await user.type(screen.getByLabelText("Why"), "Reviewed after close");
}

async function fillMemoryIdentity(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Kind"), "incident");
  await user.type(screen.getByLabelText("Subject"), "grafana datasource");
  await user.type(screen.getByLabelText("Signature"), "grafana.datasource.down");
}

function memoryAssertion(index: number): MemoryAssertion {
  return {
    id: `mem_${index}`,
    scope: { company: "acme", area: "ops" },
    agentId: "triage",
    kind: "incident",
    subject: `subject-${index}`,
    signature: `signature-${index}`,
    claim: "Refresh the datasource token before restarting the worker.",
    evidence: [
      {
        runId: `run_${index}`,
        artifact: "final_answer",
        digest: "sha256:abcd",
      },
    ],
    observations: 2,
    confirmed: 1,
    labels: ["untrusted", "scope:acme/ops"],
    status: "active",
    createdBy: "usr_ana",
    createdAt: "2026-08-25T12:00:00Z",
    updatedBy: "usr_ana",
    updatedAt: "2026-08-25T12:00:00Z",
  };
}

function memorySuggestion(index: number): MemorySuggestion {
  return {
    id: `suggestion_${index}`,
    assertionId: `mem_${index}`,
    scope: { company: "acme", area: "ops" },
    agentId: "triage",
    kind: "incident",
    subject: `suggested-subject-${index}`,
    signature: `suggested-signature-${index}`,
    claim: "Try the known datasource-token remediation first.",
    evidence: [
      {
        runId: `run_s_${index}`,
        artifact: "memory_suggestion",
        digest: "sha256:bcde",
      },
    ],
    observations: 2,
    labels: ["untrusted", "scope:acme/ops"],
    status: "pending",
    createdBy: "agent:triage",
    createdAt: "2026-08-25T12:00:00Z",
    updatedBy: "agent:triage",
    updatedAt: "2026-08-25T12:00:00Z",
  };
}

function renderMemoryPageWithClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryPage />
    </QueryClientProvider>,
  );
}

function stubMemoryAuthoringNetwork() {
  vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
    const path = new URL(request.url).pathname;
    if (path.endsWith("/runs/run_1")) {
      return json({
        runId: "run_1",
        agentId: "triage",
        scope: { company: "acme", area: "ops" },
      });
    }
    if (path.endsWith("/memory/match")) return json({});
    throw new Error(`unexpected request: ${path}`);
  }));
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
