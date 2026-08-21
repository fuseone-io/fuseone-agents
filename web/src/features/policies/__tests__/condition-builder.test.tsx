import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConditionBuilder } from "@/features/policies/condition-builder";
import { EffectSection } from "@/features/policies/effect-section";
import { IdentitySection } from "@/features/policies/identity-section";
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

  it("lets policy effect cards shrink inside the editor", () => {
    setLocale("en-US");
    const draft: PolicyInput = {
      name: "Guard writes",
      resource: "grafana.query_prometheus",
      effects: ["read"],
      conditions: [],
      effect: "deny",
      mode: "monitor",
    };

    render(<EffectSection draft={draft} patch={vi.fn()} />);

    const allow = screen.getByRole("radio", { name: /Allow/ });
    expect(allow.className).toContain("min-w-0");
    expect(allow.parentElement?.className).toContain("minmax(0,1fr)");
  });

  it("lets the policy identity grid shrink around long codes", () => {
    setLocale("en-US");
    const draft: PolicyInput = {
      name: "Block broad observability queries",
      resource: "grafana.query_prometheus",
      effects: ["read"],
      conditions: [],
      effect: "deny",
      mode: "monitor",
    };

    render(
      <IdentitySection
        draft={draft}
        patch={vi.fn()}
        code="never-run-prometheus-query-without-indexable-labels"
        editable
        onCode={vi.fn()}
      />,
    );

    const code = screen.getByLabelText("Code");
    const labelled = code.closest("div");
    expect(labelled?.parentElement?.className).toContain("min-w-0");
    expect(labelled?.parentElement?.className).toContain("minmax(0,1fr)");
  });
});
