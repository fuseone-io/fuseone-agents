import { useState } from "react";
import { useTranslation } from "react-i18next";
import { GripVertical, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { labelOf, type Block } from "@/features/agents/instruction-blocks";
import { CiteTool } from "@/features/agents/cite-tool";
import { InstructionProse } from "@/features/agents/instruction-prose";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * One block: its label in the margin, its prose in the column.
 *
 * The label is a label and not a field — hierarchy every well-written prompt
 * already has, made visible so it can be moved. Prose is set in sans at a
 * readable measure; mono is for identifiers, and a whole field of it treats
 * what an auditor reads as configuration.
 */
export function InstructionRow({
  block,
  onChange,
  onRemove,
  tools,
}: {
  block: Block;
  onChange: (text: string) => void;
  onRemove: () => void;
  tools: { catalogue: Tool[]; policies: Policy[] };
}) {
  const { t, i18n } = useTranslation();
  const [writing, setWriting] = useState(false);
  const [citing, setCiting] = useState(false);
  const label = labelOf(block.kind, i18n.language);

  /*
  `@` opens the catalogue, and picking one writes the identifier in place of
  it. The `@` never reaches the payload: it is the gesture, not the text.
  */
  const typed = (next: string) => {
    onChange(next);
    if (next.endsWith("@")) setCiting(true);
  };

  const cite = (tool: string) => {
    onChange(`${block.text.replace(/@$/, "")}${tool}`);
    setCiting(false);
  };

  return (
    <div className="group grid grid-cols-[104px_minmax(0,68ch)_auto] items-start gap-x-5 rounded-md py-2.5 transition-colors hover:bg-surface-hover">
      <span
        className={cn(
          "pt-[3px] text-right text-[10px]/5 font-medium uppercase tracking-label",
          block.kind === "never" ? "text-danger" : "text-muted-foreground",
        )}
      >
        {label || t("agents.blockProse")}
      </span>

      {/* Written in a textarea and read as prose, swapped on focus.

          The alternative is a contenteditable surface with chips inside it,
          which owns the caret, the paste and the undo stack — three things a
          browser already does correctly and a hand-written editor gets wrong
          on the day somebody pastes a paragraph from a document. What the
          chips are is a rendering, so nothing is lost by rendering them when
          nobody is typing. */}
      {writing ? (
        <CiteTool
          open={citing}
          catalogue={tools.catalogue}
          onPick={cite}
          onClose={() => setCiting(false)}
        >
          <Textarea
            autoFocus
            value={block.text}
            onChange={(e) => typed(e.target.value)}
            onBlur={() => !citing && setWriting(false)}
            placeholder={t("agents.blockPlaceholder")}
            aria-label={label || t("agents.blockProse")}
            rows={Math.max(2, block.text.split("\n").length + 1)}
            className="resize-none border-0 bg-transparent p-0 text-base/[1.65] shadow-none text-pretty focus-visible:ring-0"
          />
        </CiteTool>
      ) : (
        <div
          role="textbox"
          tabIndex={0}
          aria-label={label || t("agents.blockProse")}
          onFocus={() => setWriting(true)}
          onClick={() => setWriting(true)}
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
      )}

      <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
        <span className="cursor-grab text-text-disabled" aria-hidden>
          <GripVertical className="size-4" />
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7"
          onClick={onRemove}
          aria-label={t("agents.removeBlock")}
        >
          <Trash2 className="size-3.5" aria-hidden />
        </Button>
      </div>
    </div>
  );
}
