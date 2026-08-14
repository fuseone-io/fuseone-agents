import type { AgentDefinition } from "@/lib/api/client";

export type EditorTab = "definition" | "steps" | "tools" | "governance";

export const TABS: EditorTab[] = ["definition", "steps", "tools", "governance"];

/** The label and the one line naming what the tab decides. */
export const LABELS: Record<EditorTab, string> = {
  definition: "agents.tabDefinition",
  steps: "agents.tabSteps",
  tools: "agents.tabTools",
  governance: "agents.tabGovernance",
};

export const HINTS: Record<EditorTab, string> = {
  definition: "agents.hintDefinition",
  steps: "agents.hintSteps",
  tools: "agents.hintTools",
  governance: "agents.hintGovernance",
};

/**
 * What each tab is carrying, counted from the draft.
 *
 * Derived and never authored: the count is the reason the tab bar is worth
 * having, because it says where the substance is before anybody clicks. A
 * number somebody typed would eventually be a number that lies.
 */
export function counts(draft: AgentDefinition): Record<EditorTab, number> {
  return {
    // Two cards: who the agent is, and what it is told to do.
    definition: 2,
    steps: (draft.steps ?? []).length,
    tools: (draft.tools ?? []).length,
    // Who fires it, how far it goes, and the reading somebody approves.
    governance: 3,
  };
}
