import { beforeEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { StepRow } from "@/features/runs/step-row";
import { verbOf } from "@/features/runs/step-verb";
import { setLocale } from "@/i18n";
import type { Step } from "@/lib/api/client";

const step = (over: Partial<Step> = {}): Step => ({
  seq: 3,
  kind: "gate_decided",
  at: "2026-08-11T12:00:04Z",
  hash: "sha256:0123456789abcdef",
  ...over,
});

describe("what a step says it did", () => {
  beforeEach(() => {
    setLocale("pt-BR");
  });

  it("reads the Gate's verdict rather than the step kind", () => {
    // "decided" tells a reader nothing; "blocked" tells them what happened.
    expect(verbOf(step({ payload: { verdict: 4 } })).verb).toBe(
      "runs.verbBlocked",
    );
    expect(verbOf(step({ payload: { verdict: 1 } })).verb).toBe(
      "runs.verbAllowed",
    );
  });

  it("falls back to the kind when the verdict is missing", () => {
    expect(verbOf(step({ payload: {} })).verb).toBe("runs.verbDecided");
  });

  it("names the rule in words, never 'denied by policy'", () => {
    render(
      <ul>
        <StepRow step={step({ payload: { verdict: 4, rule: "taint" } })} last />
      </ul>,
    );
    expect(screen.getByText(/dado não confiável/i)).toBeInTheDocument();
    expect(screen.getByText("bloqueou")).toBeInTheDocument();
  });

  it("explains a data barrier block instead of rendering the raw rule", () => {
    render(
      <ul>
        <StepRow
          step={step({ payload: { verdict: 4, rule: "data_barrier" } })}
          last
        />
      </ul>,
    );

    expect(screen.getByText(/fora desta empresa ou área/i)).toBeInTheDocument();
    expect(screen.queryByText("data_barrier")).not.toBeInTheDocument();
  });

  it("translates the verb into the interface language", () => {
    setLocale("en-US");

    render(
      <ul>
        <StepRow step={step({ payload: { verdict: 4 } })} last />
      </ul>,
    );

    expect(screen.getByText("blocked")).toBeInTheDocument();
    expect(screen.queryByText("bloqueou")).not.toBeInTheDocument();
  });

  it("draws no connecting line after the last step", () => {
    const { container } = render(
      <ul>
        <StepRow step={step()} last />
      </ul>,
    );
    // The spine is what makes the order legible; a tail hanging off the end
    // suggests a step that has not arrived.
    expect(container.querySelectorAll("span.w-px")).toHaveLength(0);
  });
});
