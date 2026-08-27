import { useEffect, useRef } from "react";
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
  pendingCaret,
  onCaretApplied,
  cite,
}: {
  block: Block;
  writing: boolean;
  onWriting: (writing: boolean) => void;
  tools: { catalogue: Tool[]; policies: Policy[]; enabled?: string[] };
  typed: (text: string, cursor?: number) => void;
  pendingCaret: number | null;
  onCaretApplied: () => void;
  cite: { open: boolean; onPick: (tool: string) => void; onClose: () => void };
}) {
  const { t, i18n } = useTranslation();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const focusedWriting = useRef(false);
  const label = labelOf(block.kind, i18n.language) || t("agents.blockProse");
  const citable =
    block.kind === "howToAct"
      ? citableTools(tools.catalogue, tools.enabled)
      : tools.catalogue;

  useEffect(() => {
    if (!writing) {
      focusedWriting.current = false;
      return;
    }
    if (cite.open) return;
    const textarea = textareaRef.current;
    if (!textarea) return;
    textarea.focus();
    if (pendingCaret !== null) {
      const at = Math.min(pendingCaret, textarea.value.length);
      textarea.setSelectionRange(at, at);
      focusedWriting.current = true;
      onCaretApplied();
      return;
    }
    if (!focusedWriting.current) {
      const end = textarea.value.length;
      textarea.setSelectionRange(end, end);
      focusedWriting.current = true;
    }
  }, [writing, cite.open, pendingCaret, onCaretApplied]);

  if (!writing) {
    return (
      <div
        role="textbox"
        tabIndex={0}
        aria-label={label}
        onFocus={() => onWriting(true)}
        onClick={() => onWriting(true)}
        className="min-w-0 cursor-text rounded-sm focus-visible:outline-2 focus-visible:outline-ring"
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
      catalogue={citable}
      onPick={cite.onPick}
      onClose={cite.onClose}
    >
      <Textarea
        ref={textareaRef}
        value={block.text}
        onChange={(e) => typed(e.target.value, e.target.selectionStart)}
        onBlur={() => !cite.open && onWriting(false)}
        placeholder={t("agents.blockPlaceholder")}
        aria-label={label}
        rows={Math.max(2, block.text.split("\n").length + 1)}
        className="min-w-0 resize-none border-0 bg-transparent p-0 text-base/[1.65] break-words shadow-none text-pretty focus-visible:ring-0"
      />
    </CiteTool>
  );
}

function citableTools(catalogue: Tool[], enabled?: string[]): Tool[] {
  if (!enabled) return catalogue;
  const pack = new Set(enabled);
  return catalogue.filter((tool) => pack.has(tool.toolId));
}
