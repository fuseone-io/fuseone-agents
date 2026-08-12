import { useState } from "react";
import type { AgentDefinition, AgentDetail } from "@/lib/api/client";

/** A new agent starts with nothing chosen and nothing granted. */
export const BLANK: AgentDefinition = {
  name: "",
  area: "",
  provider: "",
  model: "",
  instructions: "",
  tools: [],
  budget: { micros: 500_000, steps: 60 },
  triggers: [],
};

/** Everything the form owns, and what differs from what was loaded. */
export function useAgentDraft(loaded?: AgentDetail) {
  const [draft, setDraft] = useState<AgentDefinition>(() => toDefinition(loaded) ?? BLANK);
  const patch = (over: Partial<AgentDefinition>) => setDraft((d) => ({ ...d, ...over }));

  const original = toDefinition(loaded);
  return { draft, patch, changes: original ? changesBetween(original, draft) : [] };
}

/** Strips a published version back to what the form edits. */
export function toDefinition(detail?: AgentDetail): AgentDefinition | undefined {
  if (!detail) return undefined;
  const { agent, instructions } = detail;
  return {
    name: agent.name,
    area: agent.scope.area,
    provider: agent.provider,
    model: agent.model,
    effort: agent.effort,
    instructions: instructions ?? "",
    tools: agent.tools,
    budget: agent.budget,
    triggers: agent.triggers ?? [],
  };
}

export interface Change {
  field: string;
  from: string;
  to: string;
}

/**
 * What publishing will change.
 *
 * Shown because publishing writes a version that runs will be pinned to, and
 * somebody should see that they widened the tool pack as part of what they are
 * about to do — rather than from the run that used it.
 */
export function changesBetween(before: AgentDefinition, after: AgentDefinition): Change[] {
  const changes: Change[] = [];
  const compare = (field: string, from: unknown, to: unknown) => {
    const left = render(from);
    const right = render(to);
    if (left !== right) changes.push({ field, from: left, to: right });
  };

  compare("nome", before.name, after.name);
  compare("área", before.area, after.area);
  compare("modelo", `${before.provider}/${before.model}`, `${after.provider}/${after.model}`);
  compare("esforço", before.effort, after.effort);
  compare("instruções", summarise(before.instructions), summarise(after.instructions));
  compare("ferramentas", before.tools, after.tools);
  compare("teto", before.budget, after.budget);
  compare("gatilhos", before.triggers, after.triggers);
  return changes;
}

/**
 * Instructions are compared by length rather than quoted.
 *
 * A diff line carrying two paragraphs of prose is unreadable in a side rail,
 * and the fact worth surfacing is that the text changed at all — the text
 * itself is on the screen already.
 */
function summarise(instructions: string): string {
  return instructions.trim() === "" ? "—" : `${instructions.trim().length} caracteres`;
}

function render(value: unknown): string {
  if (value === undefined || value === null || value === "") return "—";
  if (Array.isArray(value)) {
    return value.length === 0 ? "—" : value.map(render).join(", ");
  }
  if (typeof value === "object") {
    return Object.entries(value as Record<string, unknown>)
      .filter(([, v]) => v !== undefined && v !== 0 && v !== "")
      .map(([k, v]) => `${k} ${String(v)}`)
      .join(" ");
  }
  return String(value);
}
