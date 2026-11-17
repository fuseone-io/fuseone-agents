import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { InstructionsDiffView } from "@/features/agents/instructions-diff-view";
import type { DraftBlock, DraftOperation } from "@/lib/api/client";

/**
 * What a proposal would change, read before it is accepted. A block's is its
 * difference against the block as the draft holds it now; any other's is the
 * value it sets, by name.
 */
export function ProposalChange({ operation, blocks }: { operation: DraftOperation; blocks: DraftBlock[] }) {
  const { t } = useTranslation();
  const block = blocks.find((b) => b.id === operation.blockId);
  const heading = (b?: { label?: string }) => (b?.label ? `${b.label}\n` : "");

  switch (operation.kind) {
    case "insert_block":
      return <InstructionsDiffView was="" now={operation.text ?? ""} />;
    case "replace_block":
      return <InstructionsDiffView was={`${heading(block)}${block?.text ?? ""}`} now={`${heading(block)}${operation.text ?? ""}`} />;
    case "remove_block":
      return <InstructionsDiffView was={`${heading(block)}${block?.text ?? ""}`} now="" />;
    case "move_block":
      return <p className="text-sm">{t("workbench.conversation.moveTo", { at: (operation.to ?? 0) + 1 })}</p>;
  }
  const value = summaryOf(operation);
  return value ? <Mono className="text-sm">{value}</Mono> : null;
}

/** The value a field operation sets, as one line, or nothing to show. */
function summaryOf(op: DraftOperation): string {
  if (op.tool) return op.tool;
  if (op.model) return [op.provider, op.model, op.effort].filter(Boolean).join(" · ");
  if (op.replyLocale) return op.replyLocale;
  if (op.cachePrefixTtl) return op.cachePrefixTtl;
  if (op.stepName) return op.stepName;
  if (op.step) return op.step.name;
  if (op.fallback) return `${op.fallback.provider} · ${op.fallback.model}`;
  return "";
}
