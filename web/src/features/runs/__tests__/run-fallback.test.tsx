import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RunFailureNotice } from "@/features/runs/run-failure-notice";
import { RunFallbackNotice } from "@/features/runs/run-fallback-notice";
import { detailOf } from "@/features/runs/step-story";
import type { Run, Step } from "@/lib/api/client";

const run: Run = {
  runId: "run-1",
  scope: { company: "acme", area: "cx" },
  agentId: "triage",
  versionId: "v1",
  phase: "finished",
  seq: 9,
  startedAt: "2026-10-30T12:00:00Z",
  cost: { micros: 0 },
};

const fellBack: Run = {
  ...run,
  fallback: {
    from: { provider: "anthropic", model: "claude-opus-5" },
    to: { provider: "openai", model: "gpt-test" },
    code: "model_provider_overloaded",
    atSeq: 4,
  },
};

/*
 * A run that fell back is evidence about a pair the agent was not promoted on.
 * The page has to say so, or it reads like any other run of that version.
 */
describe("a run that switched to its fallback", () => {
  it("names both pairs and why it switched", () => {
    render(<RunFallbackNotice run={fellBack} />);

    expect(screen.getByText(/anthropic\/claude-opus-5/)).toBeInTheDocument();
    expect(screen.getByText(/openai\/gpt-test/)).toBeInTheDocument();
    expect(screen.getByText(/sobrecarregado/i)).toBeInTheDocument();
  });

  it("says nothing for a run that never switched", () => {
    const { container } = render(<RunFallbackNotice run={run} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("tells the switch apart in the trail", () => {
    const line = detailOf({
      seq: 4,
      kind: "model_fallback",
      at: "2026-10-30T12:00:00Z",
      hash: "h",
      payload: {
        from: { provider: "anthropic", model: "claude-opus-5" },
        to: { provider: "openai", model: "gpt-test" },
        failure: { code: "model_rate_limited" },
      },
    } as Step);

    expect(line).toEqual({
      key: "runs.storyFellBackDetail",
      values: { from: "anthropic/claude-opus-5", to: "openai/gpt-test", code: "model_rate_limited" },
    });
  });
});

describe("an autonomous run whose fallback was withheld", () => {
  it("leads with the withheld fallback, not only the provider", () => {
    render(
      <RunFailureNotice
        run={{
          ...run,
          phase: "parked",
          failure: {
            code: "model_provider_overloaded",
            provider: "anthropic",
            status: 529,
            reason: "fallback_unrehearsed",
          },
        }}
      />,
    );

    expect(screen.getByText(/fallback não ensaiado/i)).toBeInTheDocument();
    expect(screen.getByText(/anthropic retornou 529/i)).toBeInTheDocument();
  });

  it("names the reason on the parking in the trail", () => {
    const line = detailOf({
      seq: 5,
      kind: "parked",
      at: "2026-10-30T12:00:00Z",
      hash: "h",
      payload: { reason: "fallback_unrehearsed" },
    } as Step);

    expect(line.key).toBe("runs.storyFallbackUnrehearsed");
  });
});
