import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentStandingToggle } from "@/features/agents/agent-standing-toggle";
import { setLocale } from "@/i18n";
import type { AgentDefinition } from "@/lib/api/client";

const base = {
  name: "Developer",
  company: "default",
  area: "factory",
  provider: "anthropic",
  model: "claude-opus-5",
  instructions: "",
  tools: [],
  budget: { micros: 500_000 },
  triggers: [],
} as unknown as AgentDefinition;

function renderToggle(draft: Partial<AgentDefinition>) {
  const patch = vi.fn();
  render(<AgentStandingToggle draft={{ ...base, ...draft }} patch={patch} />);
  return patch;
}

/*
 * Publishing writes the draft whole: a setting the draft drops is a setting
 * deleted on the next edit, and every grant would release the agent again.
 */
describe("the standing approvals toggle", () => {
  beforeEach(() => setLocale("en-US"));

  it("forbids standing approvals when switched on", async () => {
    const patch = renderToggle({});
    await userEvent.click(
      screen.getByRole("switch", { name: "Only a person releases its approvals" }),
    );
    expect(patch).toHaveBeenCalledWith({ standingApprovals: "forbidden" });
  });

  // Absent rather than "allowed": the file then carries no line its author
  // never typed, and its digest stays the version it was.
  it("allows them again by leaving the setting absent", async () => {
    const patch = renderToggle({ standingApprovals: "forbidden" });
    const toggle = screen.getByRole("switch", {
      name: "Only a person releases its approvals",
    });
    expect(toggle).toBeChecked();
    await userEvent.click(toggle);
    expect(patch).toHaveBeenCalledWith({ standingApprovals: undefined });
  });
});
