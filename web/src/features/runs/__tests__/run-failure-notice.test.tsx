import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RunFailureNotice } from "@/features/runs/run-failure-notice";
import type { Run } from "@/lib/api/client";

function runWithFailure(
  overrides: Partial<NonNullable<Run["failure"]>> = {},
): Run {
  return {
    runId: "run-1",
    scope: { company: "acme", area: "cx" },
    agentId: "triage",
    versionId: "v1",
    phase: "parked",
    seq: 4,
    startedAt: "2026-08-18T12:00:00Z",
    cost: { micros: 0 },
    failure: {
      code: "model_provider_overloaded",
      provider: "anthropic",
      status: 529,
      requestId: "req_011CeAaYZkdUe63yaSu5CxCX",
      retryable: true,
      ...overrides,
    },
  };
}

describe("a run parked by a model provider failure", () => {
  it("shows the stable cause and request id instead of the raw provider text", () => {
    render(<RunFailureNotice run={runWithFailure()} />);

    expect(
      screen.getByText("Provedor de modelo sobrecarregado"),
    ).toBeInTheDocument();
    expect(screen.getByText(/anthropic retornou 529/i)).toBeInTheDocument();
    expect(screen.getByText("req_011CeAaYZkdUe63yaSu5CxCX")).toBeInTheDocument();
    expect(screen.queryByText(/overloaded_error/i)).not.toBeInTheDocument();
  });

  it("names an unknown provider without showing an empty field", () => {
    render(
      <RunFailureNotice
        run={runWithFailure({ provider: undefined, status: undefined })}
      />,
    );

    expect(
      screen.getByText(/provedor desconhecido retornou sem status/i),
    ).toBeInTheDocument();
  });
});
