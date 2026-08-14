import { segments } from "@/features/agents/instruction-entities";
import type { Block } from "@/features/agents/instruction-blocks";
import type { Policy, Tool } from "@/lib/api/client";

export interface Summary {
  /** Distinct tools the prose names, whatever the platform will do with them. */
  tools: number;
  /** Blocks that say where the agent gives up. */
  stops: number;
  /** Characters of payload. Not tokens: see below. */
  characters: number;
}

/**
 * What the instruction amounts to, counted from the instruction.
 *
 * Derived and never authored, which is the only way a count stays true — a
 * number somebody typed is a number that is wrong by the second edit.
 *
 * Characters rather than tokens, and that is a limitation stated rather than
 * hidden. A token count needs the tokeniser of the model that will read it,
 * and this console has none; printing characters under the word "tokens"
 * would put a wrong number exactly where somebody goes to estimate a cost.
 */
export function summarise(
  blocks: Block[],
  catalogue: Tool[],
  policies: Policy[],
  payload: string,
): Summary {
  const named = new Set<string>();

  for (const block of blocks) {
    for (const segment of segments(block.text, catalogue, policies)) {
      if (segment.kind === "tool" || segment.kind === "denied") {
        named.add(segment.text);
      }
    }
  }

  return {
    tools: named.size,
    stops: blocks.filter(
      (block) => block.kind === "whenToStop" && block.text.trim() !== "",
    ).length,
    characters: payload.trim().length,
  };
}
