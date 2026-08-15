import { segments } from "@/features/agents/instruction-entities";
import { ruleFor } from "@/features/agents/tool-rule";
import type { Block } from "@/features/agents/instruction-blocks";
import type { Policy, Tool } from "@/lib/api/client";

export interface Finding {
  /** Which block said it, because that is where it has to be answered. */
  at: number;
  tool: string;
  /** Why the sentence cannot come true, which is also which exit fixes it. */
  why: "refused" | "notEnabled";
  /** The rule that already refuses it, when a policy did rather than the ladder. */
  because?: string;
}

/**
 * Where the prose promises what will not happen.
 *
 * A pure function over what the text names, the policies in force and what
 * this agent holds. Two rules, and both are the same sentence: the text says
 * the agent will do something, and it will not. The person who finds that out
 * otherwise is whoever reads the run afterwards wondering why it stopped.
 *
 * They are findings and never refusals. Forbidding in prose what the platform
 * blocks anyway is redundant rather than wrong, and the text may be explaining
 * the rule to whoever reads the definition next — which is a good reason to
 * keep it, and the author's to give.
 */
export function findings(
  blocks: Block[],
  catalogue: Tool[],
  policies: Policy[],
  enabled: string[],
): Finding[] {
  const out: Finding[] = [];
  const held = new Set(enabled);

  blocks.forEach((block, at) => {
    for (const segment of segments(block.text, catalogue, policies)) {
      const finding = readingOf(segment, at, catalogue, policies, held);
      if (finding) out.push(finding);
    }
  });

  return out;
}

/**
 * What one named thing amounts to, or nothing.
 *
 * A refusal is reported ahead of a missing checkbox, and never both. Both can
 * be true of the same tool, and only one of them is worth saying: enabling a
 * tool a policy denies changes nothing, so offering that as the fix would be
 * an exit that leads nowhere.
 */
function readingOf(
  segment: { kind: string; text: string },
  at: number,
  catalogue: Tool[],
  policies: Policy[],
  held: Set<string>,
): Finding | undefined {
  if (segment.kind === "denied") {
    // The rule that produced the refusal comes from the same function the
    // tool rows use, so a policy cited here and a verdict shown there cannot
    // disagree about which one decided.
    const effect =
      catalogue.find((one) => one.toolId === segment.text)?.effect ?? "write";
    return {
      at,
      tool: segment.text,
      why: "refused",
      because: ruleFor(segment.text, effect, policies).because,
    };
  }

  // A tool the platform allows and this agent was never given. The pack a run
  // is offered is the agent's enabled tools, so one outside it cannot even be
  // proposed: the sentence describes a step that does not happen, and nothing
  // in the trail afterwards says why.
  if (segment.kind === "tool" && !held.has(segment.text)) {
    return { at, tool: segment.text, why: "notEnabled" };
  }

  return undefined;
}
