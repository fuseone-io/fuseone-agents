import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { DecisionCard } from "@/features/approvals/decision-card";
import { riskOf } from "@/features/approvals/risk";
import type { PendingApproval } from "@/lib/api/client";

const item = (over: Partial<PendingApproval> = {}): PendingApproval => ({
  runId: "run_a4d76",
  agentId: "triage",
  tool: "crm.note",
  rule: "taint",
  atSeq: 7,
  requestedAt: new Date(Date.now() - 120_000).toISOString(),
  scope: { company: "acme", area: "cx" },
  ...over,
});

describe("a decision card", () => {
  it("names the rule that stopped the run, never 'denied by policy'", () => {
    render(<DecisionCard item={item()} selected={false} onSelect={() => {}} />);
    expect(screen.getByText(/dado não confiável/i)).toBeInTheDocument();
  });

  it("states the risk in words, so colour is not carrying it alone", () => {
    render(
      <DecisionCard
        item={item({ effect: "financial" })}
        selected={false}
        onSelect={() => {}}
      />,
    );
    expect(screen.getByText("Risco alto")).toBeInTheDocument();
  });
});

describe("risk", () => {
  it("comes from what the Curator classified the tool as", () => {
    // Not an invented scale: the classification is already the basis on which
    // the Gate decided to stop and ask.
    expect(riskOf("financial")).toBe("high");
    expect(riskOf("destructive")).toBe("high");
    expect(riskOf("write")).toBe("medium");
    expect(riskOf("read")).toBe("low");
  });

  it("says it does not know rather than guessing low", () => {
    expect(riskOf(undefined)).toBe("unknown");
  });
});
