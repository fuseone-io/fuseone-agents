import { ruleFor } from "@/features/agents/tool-rule";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * What the platform understood, as sentences a person can approve.
 *
 * FU-08 asks the system to read back what it understood before anything is
 * published, and to have the author approve prose rather than YAML, JSON or a
 * graph.
 *
 * Derived, never written by a model. What the platform "understood" is the
 * specification's fields plus what the Gate will actually do with each tool —
 * both of which it knows exactly. A model paraphrasing them would produce
 * fluent sentences that the platform has not promised, and the author would be
 * approving those. It also makes the read-back reproducible, free, and
 * available to an installation with no model at all.
 */
export interface NarrativeLine {
  key: string;
  values?: Record<string, string | number>;
}

const EFFECT_OF = (tools: Tool[], name: string): string =>
  tools.find((t) => t.toolId === name)?.effect ?? "unknown";

export function narrate(
  draft: AgentDefinition,
  tools: Tool[],
  policies: Policy[],
): NarrativeLine[] {
  const lines: NarrativeLine[] = [];

  lines.push(startedBy(draft));

  const ruled = (draft.tools ?? []).map((name) => ({
    name,
    rule: ruleFor(name, EFFECT_OF(tools, name), policies),
  }));

  const reads = ruled
    .filter((t) => t.rule.kind === "allowed")
    .map((t) => t.name);
  const asks = ruled.filter((t) => t.rule.kind === "asks").map((t) => t.name);
  const blocked = ruled
    .filter((t) => t.rule.kind === "blocked")
    .map((t) => t.name);

  if (reads.length > 0)
    lines.push({ key: "narrative.reads", values: { tools: reads.join(", ") } });
  // The two that matter most, and they are stated even when empty: "it can
  // act without asking anybody" is exactly what an approver needs to be told.
  lines.push(
    asks.length > 0
      ? { key: "narrative.asks", values: { tools: asks.join(", ") } }
      : { key: "narrative.asksNobody" },
  );
  if (blocked.length > 0) {
    lines.push({
      key: "narrative.never",
      values: { tools: blocked.join(", ") },
    });
  }
  if (ruled.length === 0) lines.push({ key: "narrative.noTools" });

  lines.push(bounded(draft));
  return lines;
}

/** What opens a run, in the author's terms rather than the trigger's. */
function startedBy(draft: AgentDefinition): NarrativeLine {
  const triggers = draft.triggers ?? [];
  if (triggers.length === 0) return { key: "narrative.startedByHand" };

  const first = triggers[0];
  if (first?.type === "cron" && first.schedule) {
    return {
      key: "narrative.startedOnSchedule",
      values: { schedule: first.schedule },
    };
  }
  if (first?.type === "webhook" && first.path) {
    return { key: "narrative.startedByWebhook", values: { path: first.path } };
  }
  return {
    key: "narrative.startedByEvent",
    values: { event: first?.event ?? "" },
  };
}

/**
 * The ceiling, said as what happens when it is reached rather than as a
 * number. "It pauses and waits" is the fact an author has to agree to.
 */
function bounded(draft: AgentDefinition): NarrativeLine {
  const micros = draft.budget?.micros ?? 0;
  const steps = draft.budget?.steps ?? 0;
  if (micros > 0 && steps > 0)
    return { key: "narrative.boundedBoth", values: { micros, steps } };
  if (micros > 0) return { key: "narrative.boundedCost", values: { micros } };
  if (steps > 0) return { key: "narrative.boundedSteps", values: { steps } };
  return { key: "narrative.unbounded" };
}
