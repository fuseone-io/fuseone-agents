import { useState } from "react";
import type { AgentDefinition, AgentDetail } from "@/lib/api/client";

/** A new agent starts with nothing chosen and nothing granted. */
export const BLANK: AgentDefinition = {
  name: "",
  company: "",
  area: "",
  provider: "",
  model: "",
  instructions: "",
  tools: [],
  budget: { micros: 500_000, steps: 60 },
  triggers: [],
};

/**
 * Everything the form owns, and what differs from what was loaded.
 *
 * Seeded once, when the agent arrives — which is not always the first render.
 * Opening /agents/{id}/edit cold renders before the query resolves, so a draft
 * built only in the initialiser stayed blank: the screen showed an empty form
 * for a real agent, and publishing from it would have replaced the definition
 * with empty fields. Navigating in from the list hid this, because the agent
 * was already in the cache.
 *
 * Seeded once and no more, so a refetch does not overwrite what somebody is
 * halfway through typing.
 */
export function useAgentDraft(loaded?: AgentDetail) {
  const original = toDefinition(loaded);
  const [draft, setDraft] = useState<AgentDefinition>(
    () => original ?? fromInterview() ?? BLANK,
  );
  const [seeded, setSeeded] = useState(original !== undefined);

  if (!seeded && original) {
    // Adjusting state during render rather than in an effect: React discards
    // this pass and re-renders with the value, so the blank form is never
    // shown and never flashes.
    setSeeded(true);
    setDraft(original);
  }

  const patch = (over: Partial<AgentDefinition>) =>
    setDraft((d) => ({ ...d, ...over }));
  return {
    draft,
    patch,
    changes: original ? changesBetween(original, draft) : [],
    // The published prose, so the editor can show what publishing would
    // change in it. Absent while creating: there is nothing to compare to,
    // and "everything is new" is not a review.
    published: original?.instructions,
  };
}

/**
 * A published version, as the form's draft.
 *
 * Everything it holds travels, including the parts no field on this screen
 * shows. Publishing writes the draft whole, so a value dropped here is a value
 * deleted on the next edit — which is how an agent that emitted an event
 * quietly stopped, taking every agent composed onto it with it.
 */
export function toDefinition(
  detail?: AgentDetail,
): AgentDefinition | undefined {
  if (!detail) return undefined;
  const { agent, instructions, steps, emits } = detail;
  return {
    name: agent.name,
    company: agent.scope.company,
    area: agent.scope.area,
    provider: agent.provider,
    model: agent.model,
    effort: agent.effort,
    instructions: instructions ?? "",
    tools: agent.tools,
    budget: agent.budget,
    triggers: agent.triggers ?? [],
    memoryLearning: agent.memoryLearning,
    // Everything the version holds, and not only what the form shows a field
    // for. What is not carried here is deleted the next time somebody
    // publishes — silently, on an edit they made for another reason.
    steps: steps ?? [],
    emits: emits ?? [],
  };
}

export interface Change {
  field: string;
  from: string;
  to: string;
  /**
   * Whether the two sides are worth printing.
   *
   * Prose is not: two paragraphs on a diff line in a side rail is unreadable,
   * and what actually changed in it is a word-level view of its own. The
   * summary says the text moved; the card says how.
   */
  quiet?: boolean;
}

/**
 * What publishing will change.
 *
 * Shown because publishing writes a version that runs will be pinned to, and
 * somebody should see that they widened the tool pack as part of what they are
 * about to do — rather than from the run that used it.
 */
export function changesBetween(
  before: AgentDefinition,
  after: AgentDefinition,
): Change[] {
  const changes: Change[] = [];
  const compare = (field: string, from: unknown, to: unknown, quiet = false) => {
    const left = render(from);
    const right = render(to);
    if (left !== right) changes.push({ field, from: left, to: right, quiet });
  };

  compare("agents.fieldName", before.name, after.name);
  compare("agents.fieldArea", scopeOf(before), scopeOf(after));
  compare(
    "agents.fieldModel",
    `${before.provider}/${before.model}`,
    `${after.provider}/${after.model}`,
  );
  compare("agents.fieldEffort", before.effort, after.effort);
  compare(
    "agents.fieldInstructions",
    size(before.instructions),
    size(after.instructions),
    true,
  );
  compare("agents.fieldTools", before.tools, after.tools);
  compare("agents.fieldBudget", before.budget, after.budget);
  compare("agents.fieldTriggers", before.triggers, after.triggers);
  compare("agents.fieldMemoryLearning", before.memoryLearning, after.memoryLearning);
  // The steps are a change worth naming: they are what the Gate is meant to
  // obey, and "0 changes" on a screen where somebody just redrew the process
  // is the summary telling them their work did not land.
  compare("agents.fieldSteps", before.steps, after.steps);
  compare("agents.fieldEmits", before.emits, after.emits);
  return changes;
}

function scopeOf(definition: AgentDefinition): string {
  if (definition.company === "" && definition.area === "") return "";
  return `${definition.company}/${definition.area}`;
}

/**
 * Instructions are compared by length rather than quoted.
 *
 * Enough to know the text moved, which is all this summary claims. What moved
 * in it is the card's word-level view, and the number never reaches a screen —
 * so it needs no unit and belongs to no language.
 */
function size(instructions: string): string {
  return String(instructions.trim().length);
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

/**
 * A draft the interview left behind, read once and then forgotten.
 *
 * Handed over rather than published: nothing reaches the registry without a
 * person having read it back first (FU-08). Removed on read so that a reload
 * of the editor an hour later does not resurrect an interview somebody
 * abandoned.
 */
function fromInterview(): AgentDefinition | undefined {
  try {
    const held = globalThis.sessionStorage?.getItem("fuseone.draft");
    if (!held) return undefined;
    globalThis.sessionStorage.removeItem("fuseone.draft");
    return { ...BLANK, ...(JSON.parse(held) as Partial<AgentDefinition>) };
  } catch {
    // A browser that refuses storage still authors agents through the form.
    return undefined;
  }
}
