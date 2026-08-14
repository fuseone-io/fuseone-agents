import { useTranslation } from "react-i18next";
import { GripVertical, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { labelOf, type Block } from "@/features/agents/instruction-blocks";

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
}: {
  block: Block;
  onChange: (text: string) => void;
  onRemove: () => void;
}) {
  const { t, i18n } = useTranslation();
  const label = labelOf(block.kind, i18n.language);

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

      {/* A textarea that grows with what is in it: prose is written in
          paragraphs, and a box that scrolls at four lines hides the sentence
          somebody is about to reconsider. */}
      <Textarea
        value={block.text}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t("agents.blockPlaceholder")}
        aria-label={label || t("agents.blockProse")}
        rows={Math.max(2, block.text.split("\n").length + 1)}
        className="resize-none border-0 bg-transparent p-0 text-base/[1.65] shadow-none text-pretty focus-visible:ring-0"
      />

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
