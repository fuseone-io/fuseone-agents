import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConditionBuilder } from "@/features/policies/condition-builder";
import { EffectSection } from "@/features/policies/effect-section";
import { setLocale } from "@/i18n";
import type { PolicyInput } from "@/lib/api/client";

describe("the policy condition builder", () => {
  it("renders translated conjunctions and lets condition rows shrink", () => {
    setLocale("en-US");
    render(
      <ConditionBuilder
        conditions={[
          { field: "tool.id", op: "eq", value: "grafana.query_loki_logs" },
          { field: "args.rows", op: "lt", value: "1000" },
        ]}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByText("when")).toBeInTheDocument();
    expect(screen.getByText("and")).toBeInTheDocument();
    expect(screen.queryByText("policies.when")).not.toBeInTheDocument();

    const row = screen.getByText("when").closest("div");
    expect(row?.className).toContain("min-w-0");
    expect(row?.className).toContain("minmax(0,1fr)");
  });

  it("renders policy effect labels as words, not translation keys", () => {
    setLocale("en-US");
    const draft: PolicyInput = {
      name: "Guard writes",
      resource: "crm.*",
      effects: ["write"],
      conditions: [],
      effect: "deny",
      mode: "monitor",
    };

    render(<EffectSection draft={draft} patch={vi.fn()} />);

    expect(screen.getByRole("radio", { name: /Monitor/ })).toBeInTheDocument();
    expect(screen.getByText("records and continues")).toBeInTheDocument();
    expect(screen.queryByText("policies.monitor")).not.toBeInTheDocument();
  });
});
