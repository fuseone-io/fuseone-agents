import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryDuplicateNotice } from "@/features/memory/memory-duplicate-notice";
import type {
  MemoryAssertion,
  MemoryStatus,
  MemoryMatch,
} from "@/features/memory/api";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function assertion(status: MemoryStatus, agentId = "triage"): MemoryAssertion {
  return {
    id: "m1",
    scope: { company: "acme", area: "ops" },
    agentId,
    kind: "policy",
    subject: "refunds",
    signature: "refund.limit",
    claim: "the limit is R$ 500",
    evidence: [],
    observations: 2,
    confirmed: 2,
    labels: [],
    status,
    createdBy: "u",
    createdAt: "2026-08-01T00:00:00Z",
    updatedBy: "u",
    updatedAt: "2026-08-01T00:00:00Z",
  };
}

function renderNotice(match: MemoryMatch, reason = "reviewed today") {
  const posted: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      posted.push(new URL(url, "http://localhost").pathname);
      return new Response("{}", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const improve = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <MemoryDuplicateNotice
        match={match}
        reason={reason}
        onImproveShared={improve}
      />
    </QueryClientProvider>,
  );
  return { posted, improve };
}

describe("what the platform already holds", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("says nothing when the fact is new", () => {
    const { container } = render(
      <MemoryDuplicateNotice match={{}} reason="" onImproveShared={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  // Correcting rather than duplicating is what saving does here, and it is the
  // one thing the person cannot tell from the form alone.
  it("says an active memory will be corrected, and offers nothing else", () => {
    renderNotice({ own: assertion("active") });

    expect(screen.getByText(/já existe/i)).toBeInTheDocument();
    expect(screen.getByText(/corrige o texto dela/i)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  // The server refuses to merge into a disabled row, so saving is not the route
  // back and is not offered as one.
  it("offers to bring back a disabled memory", async () => {
    const { posted } = renderNotice({ own: assertion("disabled") });

    await userEvent.click(screen.getByRole("button", { name: /^reativar$/i }));

    expect(posted).toContain("/api/v1/admin/memory/assertions/m1/reactivate");
  });

  // The reason is what makes reactivation an act rather than a state change,
  // and the server refuses without one. A button that can only be refused is
  // not an offer.
  it("does not offer to bring one back before the reason is written", async () => {
    const { posted } = renderNotice({ own: assertion("disabled") }, "  ");

    const button = screen.getByRole("button", { name: /escreva o porquê/i });
    expect(button).toBeDisabled();
    await userEvent.click(button);
    expect(posted).toHaveLength(0);
  });

  it("says an expired memory is renewed by saving", () => {
    renderNotice({ own: assertion("expired") });

    expect(screen.getByText(/vencida/i)).toBeInTheDocument();
    expect(screen.getByText(/renova o prazo/i)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  // Nothing brings back a memory whose source was erased, and nothing should:
  // the record of the erasure is the point. Shown so the person understands
  // why teaching this again starts from nothing.
  it("shows an erased memory and offers nothing", () => {
    renderNotice({ own: assertion("source_erased") });

    expect(screen.getByText(/origem dela foi apagada/i)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  // Improving the memory every agent reads is a decision, never a side effect
  // of teaching one agent something.
  it("offers to improve the shared memory explicitly", async () => {
    const { improve } = renderNotice({ shared: assertion("active", "") });

    expect(screen.getByText(/compartilhada já responde/i)).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: /melhorar a compartilhada/i }),
    );
    expect(improve).toHaveBeenCalledOnce();
  });

  it("does not offer to improve a shared memory that is not active", () => {
    renderNotice({ shared: assertion("disabled", "") });

    expect(screen.getByText(/compartilhada já responde/i)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("names a pending proposal without offering to decide it here", () => {
    renderNotice({
      pending: {
        id: "s1",
        assertionId: "a1",
        scope: { company: "acme", area: "ops" },
        agentId: "triage",
        kind: "policy",
        subject: "refunds",
        signature: "refund.limit",
        claim: "the limit is R$ 500",
        evidence: [],
        observations: 2,
        labels: [],
        status: "pending",
        createdBy: "agent",
        createdAt: "2026-08-01T00:00:00Z",
        updatedBy: "agent",
        updatedAt: "2026-08-01T00:00:00Z",
      },
    });

    expect(screen.getByText(/proposta pendente/i)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
