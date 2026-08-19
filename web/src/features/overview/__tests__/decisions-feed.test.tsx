import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DecisionsFeed } from "@/features/overview/decisions-feed";
import { setLocale } from "@/i18n";

const overview = vi.hoisted(() => ({
  result: {
    data: {
      items: [
        {
          runId: "run_1",
          seq: 3,
          at: "2026-08-18T12:00:00Z",
          verdict: "allow",
          tool: "crm.lookup",
          agentId: "support",
        },
      ],
    },
    isLoading: false,
    error: null,
  },
}));

vi.mock("@/features/overview/api", () => ({
  useDecisions: () => overview.result,
}));

describe("overview decisions feed", () => {
  beforeEach(() => {
    setLocale("pt-BR");
  });

  it("renders verdict verbs in the current interface language", () => {
    setLocale("en-US");

    render(
      <MemoryRouter>
        <DecisionsFeed since="2026-08-18T00:00:00Z" />
      </MemoryRouter>,
    );

    expect(screen.getByText("allowed")).toBeInTheDocument();
    expect(screen.queryByText("permitiu")).not.toBeInTheDocument();
  });
});
