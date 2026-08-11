import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { PhaseBadge } from "@/features/runs/phase-badge";
import { stateOfPhase } from "@/lib/agent-state";
import type { Phase } from "@/lib/api/client";

const PHASES: Phase[] = [
  "unstarted",
  "running",
  "awaiting_tool",
  "awaiting_approval",
  "parked",
  "finished",
];

describe("the run phase badge", () => {
  it.each(PHASES)(
    "states %s in words, so the run survives a monochrome print and a colour-blind reader",
    (phase) => {
      render(<PhaseBadge phase={phase} />);
      expect(screen.getByText(/\p{L}/u)).toBeInTheDocument();
    },
  );

  it("does not announce the colour dot, which repeats what the label already says", () => {
    const { container } = render(<PhaseBadge phase="parked" />);
    expect(container.querySelectorAll('[aria-hidden="true"]')).toHaveLength(1);
  });

  it("reads a run waiting on a person as waiting, not as merely running", () => {
    expect(stateOfPhase("awaiting_approval")).toBe("waiting");
    expect(stateOfPhase("awaiting_tool")).toBe("running");
  });
});
