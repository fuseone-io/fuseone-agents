import { segments } from "@/features/agents/instruction-entities";
import { ruleFor } from "@/features/agents/tool-rule";
import type { Block } from "@/features/agents/instruction-blocks";
import type { Policy, Tool } from "@/lib/api/client";

export interface Finding {
  /** Which block said it, because that is where it has to be answered. */
  at: number;
  tool: string;
  /** The rule that already refuses it, when a policy did rather than the ladder. */
  because?: string;
}

/**
 * Where the prose promises what the platform already refuses.
 *
 * A pure function over what the text names and the policies in force. One
 * rule, deliberately: an instruction that tells the agent to do something a
 * policy denies is a sentence that will never come true, and the person who
 * finds that out is whoever reads the run afterwards wondering why it stopped.
 *
 * It is a finding and never a refusal. Forbidding in prose what the platform
 * blocks anyway is redundant rather than wrong, and the text may be
 * explaining the rule to whoever reads the definition next — which is a good
 * reason to keep it, and the author's to give.
 */
export function findings(
  blocks: Block[],
  catalogue: Tool[],
  policies: Policy[],
): Finding[] {
  const out: Finding[] = [];

  blocks.forEach((block, at) => {
    for (const segment of segments(block.text, catalogue, policies)) {
      if (segment.kind !== "denied") continue;
      // The rule that produced the refusal comes from the same function the
      // tool rows use, so a policy cited here and a verdict shown there
      // cannot disagree about which one decided.
      const effect =
        catalogue.find((one) => one.toolId === segment.text)?.effect ?? "write";
      out.push({
        at,
        tool: segment.text,
        because: ruleFor(segment.text, effect, policies).because,
      });
    }
  });

  return out;
}
