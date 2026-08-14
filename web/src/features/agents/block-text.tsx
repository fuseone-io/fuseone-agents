import { useTranslation } from "react-i18next";
import { Textarea } from "@/components/ui/textarea";
import { CiteTool } from "@/features/agents/cite-tool";
import { InstructionProse } from "@/features/agents/instruction-prose";
import { labelOf, type Block } from "@/features/agents/instruction-blocks";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * The prose of one block, written or read.
 *
 * Written in a textarea and read as prose, swapped on focus. The alternative
 * is a contenteditable surface with chips inside it, which owns the caret, the
 * paste and the undo stack — three things a browser already does correctly and
 * a hand-written editor gets wrong the first time somebody pastes from a
 * document. The chips are a rendering, so nothing is lost by rendering them
 * when nobody is typing.
 */
export function BlockText({
  block,
  writing,
  onWriting,
  tools,
  typed,
  cite,
}: {
  block: Block;
  writing: boolean;
  onWriting: (writing: boolean) => void;
  tools: { catalogue: Tool[]; policies: Policy[] };
  typed: (text: string) => void;
  cite: { open: boolean; onPick: (tool: string) => void; onClose: () => void };
}) {
  const { t, i18n } = useTranslation();
  const label = labelOf(block.kind, i18n.language) || t("agents.blockProse");

  if (!writing) {
    return (
      <div
        role="textbox"
        tabIndex={0}
        aria-label={label}
        onFocus={() => onWriting(true)}
        onClick={() => onWriting(true)}
        className="cursor-text rounded-sm focus-visible:outline-2 focus-visible:outline-ring"
      >
        {block.text.trim() === "" ? (
          <p className="text-base/[1.65] text-muted-foreground">
            {t("agents.blockPlaceholder")}
          </p>
        ) : (
          <InstructionProse
            text={block.text}
            catalogue={tools.catalogue}
            policies={tools.policies}
          />
        )}
      </div>
    );
  }

  return (
    <CiteTool
      open={cite.open}
      catalogue={tools.catalogue}
      onPick={cite.onPick}
      onClose={cite.onClose}
    >
      <Textarea
        autoFocus
        value={block.text}
        onChange={(e) => typed(e.target.value)}
        onBlur={() => !cite.open && onWriting(false)}
        placeholder={t("agents.blockPlaceholder")}
        aria-label={label}
        rows={Math.max(2, block.text.split("\n").length + 1)}
        className="resize-none border-0 bg-transparent p-0 text-base/[1.65] shadow-none text-pretty focus-visible:ring-0"
      />
    </CiteTool>
  );
}
