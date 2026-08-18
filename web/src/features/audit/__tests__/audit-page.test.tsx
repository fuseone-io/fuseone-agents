import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuditPage } from "@/features/audit/audit-page";
import type { AuditEntry } from "@/lib/api/client";

const audit = vi.hoisted(() => ({
  useAuditPage: vi.fn(),
}));

vi.mock("@/features/audit/api", () => ({
  useAuditPage: audit.useAuditPage,
}));

function entry(target: string, reason: string): AuditEntry {
  return {
    at: `2026-08-18T12:00:00.000Z`,
    source: "ledger",
    actor: "agent_triage",
    verb: "gate.allowed",
    target,
    scope: { company: "acme", area: "ops" },
    detail: { reason },
    runId: "run_1",
    seq: 1,
    hash: "abc1234567890",
  };
}

function open() {
  return render(
    <MemoryRouter>
      <AuditPage />
    </MemoryRouter>,
  );
}

describe("audit pagination", () => {
  beforeEach(() => {
    audit.useAuditPage.mockReset();
    audit.useAuditPage.mockImplementation((_filters, cursor?: string) => {
      if (cursor === "cursor-2") {
        return {
          data: {
            items: [entry("run_second", "Second page only")],
            nextCursor: null,
          },
          isLoading: false,
          error: null,
          refetch: vi.fn(),
        };
      }
      return {
        data: {
          items: [
            entry("run_first", "First page first row"),
            entry("run_first_b", "First page second row"),
          ],
          nextCursor: "cursor-2",
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      };
    });
  });

  it("moves between cursor pages instead of appending every fetched row into one card", async () => {
    open();

    expect(screen.getByText("First page first row")).toBeInTheDocument();
    expect(screen.getByText("First page second row")).toBeInTheDocument();
    expect(screen.getByText(/página 1|page 1/i)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /próxima|next/i }));

    expect(await screen.findByText("Second page only")).toBeInTheDocument();
    expect(screen.queryByText("First page first row")).not.toBeInTheDocument();
    expect(screen.queryByText("First page second row")).not.toBeInTheDocument();
    expect(screen.getByText(/página 2|page 2/i)).toBeInTheDocument();
  });
});
