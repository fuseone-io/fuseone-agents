import type { ReactNode } from "react";
import type { TFunction } from "i18next";
import { InstructionsDiffView } from "@/features/agents/instructions-diff-view";
import { fieldValue } from "@/features/workbench/proposal-value";
import type { DraftBlock, DraftOperation } from "@/lib/api/client";

/**
 * What a proposal would change, read before it is accepted, or undefined when
 * it cannot be shown. A block's is its difference against the block as the
 * draft holds it now; any other's is the value it sets.
 */
export function proposalChange(op: DraftOperation, blocks: DraftBlock[], t: TFunction): ReactNode | undefined {
  const block = blocks.find((b) => b.id === op.blockId);
  const heading = block?.label ? `${block.label}\n` : "";
  switch (op.kind) {
    case "insert_block":
      return op.text ? <InstructionsDiffView was="" now={op.text} /> : undefined;
    case "replace_block":
      return block && op.text !== undefined
        ? <InstructionsDiffView was={`${heading}${block.text}`} now={`${heading}${op.text}`} />
        : undefined;
    case "remove_block":
      return block ? <InstructionsDiffView was={`${heading}${block.text}`} now="" /> : undefined;
    case "move_block":
      return block && op.to !== undefined
        ? <p className="text-sm">{t("workbench.conversation.moveTo", { at: op.to + 1 })}</p>
        : undefined;
  }
  return fieldValue(op, t);
}
