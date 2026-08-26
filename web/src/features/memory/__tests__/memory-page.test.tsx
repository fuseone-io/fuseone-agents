import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
    useCreateMemoryAssertion: () => ({ mutateAsync: hooks.create, isPending: false }),
    useDisableMemoryAssertion: () => ({ mutate: hooks.disable, isPending: false }),
    useAcceptMemorySuggestion: () => ({ mutate: hooks.accept, isPending: false }),
    useDismissMemorySuggestion: () => ({ mutate: hooks.dismiss, isPending: false }),
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

  it("shows that more remembered assertions exist instead of cutting silently", () => {
    hooks.items = Array.from({ length: 9 }, (_, index) => memoryAssertion(index));
    render(<MemoryPage />);

    expect(screen.getByText("subject-0")).toBeInTheDocument();
    expect(screen.getByText("subject-7")).toBeInTheDocument();
    expect(screen.queryByText("subject-8")).not.toBeInTheDocument();
    expect(screen.getByText("8 of 9")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Load more" })).toBeInTheDocument();
  });

  it("records a reviewed assertion with ledger evidence", async () => {
    const user = userEvent.setup();
    render(<MemoryPage />);
    await fillAssertion(user);

    await user.click(screen.getByRole("button", { name: "Record memory" }));

    expect(hooks.create).toHaveBeenCalledWith({
      company: "acme", area: "ops", agentId: "triage", kind: "incident",
      subject: "grafana datasource", signature: "grafana.datasource.down",
      claim: "Refresh the datasource token before restarting the worker.",
      observations: 1, confirmed: 1, reason: "Reviewed after close",
      evidence: [{ runId: "run_1", artifact: "final_answer", digest: "sha256:abcd" }],
    });
  });

  it("requires a reason before disabling a remembered assertion", async () => {
    const user = userEvent.setup();
    hooks.items = [memoryAssertion(0)];
    render(<MemoryPage />);

    await user.click(screen.getByRole("button", { name: "Disable" }));
    const dialog = screen.getByRole("alertdialog");
    expect(within(dialog).getByRole("button", { name: "Disable" })).toBeDisabled();
    await user.type(within(dialog).getByLabelText("Why"), "superseded");
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));

    expect(hooks.disable.mock.calls[0]?.[0]).toEqual({
      id: "mem_0", company: "acme", area: "ops", reason: "superseded",
    });
  });

  it("reviews a pending suggestion inside its own scope", async () => {
    const user = userEvent.setup();
    hooks.suggestions = [memorySuggestion(0)];
    render(<MemoryPage />);

    await user.click(screen.getByRole("button", { name: "Accept suggestion" }));
    const dialog = screen.getByRole("alertdialog");
    expect(
      within(dialog).getByRole("button", { name: "Accept suggestion" }),
    ).toBeDisabled();
    await user.type(within(dialog).getByLabelText("Why"), "observed twice");
    await user.click(
      within(dialog).getByRole("button", { name: "Accept suggestion" }),
    );

    expect(hooks.accept.mock.calls[0]?.[0]).toEqual({
      id: "suggestion_0", company: "acme", area: "ops", reason: "observed twice",
    });
  });

  it("shows that more memory suggestions exist instead of cutting silently", () => {
    hooks.suggestions = Array.from({ length: 9 }, (_, index) =>
      memorySuggestion(index),
    );

    render(<MemoryPage />);

    expect(screen.getByText("suggested-subject-0")).toBeInTheDocument();
    expect(screen.getByText("suggested-subject-7")).toBeInTheDocument();
    expect(screen.queryByText("suggested-subject-8")).not.toBeInTheDocument();
    expect(screen.getByText("8 of 9")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Load more" })).toBeInTheDocument();
  });

  it("keeps write controls out for read-only callers", () => {
    hooks.can = ["agent:read"];
    hooks.items = [memoryAssertion(0)];
    hooks.suggestions = [memorySuggestion(0)];

    render(<MemoryPage />);

    expect(screen.getByText("Read-only memory")).toBeInTheDocument();
    expect(screen.queryByText("New memory")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Accept suggestion" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Dismiss suggestion" })).toBeNull();
  });
});

async function fillAssertion(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Agent (optional)"), "triage");
  await user.type(screen.getByLabelText("Kind"), "incident");
  await user.type(screen.getByLabelText("Subject"), "grafana datasource");
  await user.type(screen.getByLabelText("Signature"), "grafana.datasource.down");
  await user.type(screen.getByLabelText("Claim"), "Refresh the datasource token before restarting the worker.");
  await user.type(screen.getByLabelText("Run"), "run_1");
  await user.type(screen.getByLabelText("Digest"), "sha256:abcd");
  await user.type(screen.getByLabelText("Why"), "Reviewed after close");
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
    evidence: [{ runId: `run_${index}`, artifact: "final_answer", digest: "sha256:abcd" }],
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
    evidence: [{ runId: `run_s_${index}`, artifact: "memory_suggestion", digest: "sha256:bcde" }],
    observations: 2,
    labels: ["untrusted", "scope:acme/ops"],
    status: "pending",
    createdBy: "agent:triage",
    createdAt: "2026-08-25T12:00:00Z",
    updatedBy: "agent:triage",
    updatedAt: "2026-08-25T12:00:00Z",
  };
}
