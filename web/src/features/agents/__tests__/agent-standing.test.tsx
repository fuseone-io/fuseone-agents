import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentStandingToggle } from "@/features/agents/agent-standing-toggle";
import { changesBetween, useAgentDraft } from "@/features/agents/agent-draft";
import { setLocale } from "@/i18n";
import type { AgentDefinition, AgentDetail } from "@/lib/api/client";

const forbidding = {
  agent: {
    agentId: "factory-developer",
    name: "Developer",
    scope: { company: "default", area: "factory" },
    provider: "anthropic",
    model: "claude-opus-5",
    standingApprovals: "forbidden",
    tools: ["github.issue.comment"],
    budget: { micros: 500_000, steps: 60 },
  },
  instructions: "Propose the change.",
} as unknown as AgentDetail;

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
describe("standing approvals in the draft", () => {
  it("survive an edit made for another reason", () => {
    const { result } = renderHook(() => useAgentDraft(forbidding));
    act(() => result.current.patch({ instructions: "Propose it smaller." }));
    expect(result.current.draft.standingApprovals).toBe("forbidden");
  });

  it("are named among the changes publishing will make", () => {
    const after = { ...base, standingApprovals: "forbidden" as const };
    const fields = changesBetween(base, after).map((c) => c.field);
    expect(fields).toContain("agents.fieldStandingApprovals");
  });
});

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
