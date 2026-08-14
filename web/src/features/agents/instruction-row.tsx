import { useState } from "react";
import { useTranslation } from "react-i18next";
import { GripVertical, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { labelOf, type Block } from "@/features/agents/instruction-blocks";
import { withoutSentence } from "@/features/agents/without-sentence";
import { BlockText } from "@/features/agents/block-text";
import { InstructionFinding } from "@/features/agents/instruction-finding";
import type { Finding } from "@/features/agents/instruction-lint";
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
  at,
  on,
  tools,
  findings,
}: {
  block: Block;
  /** Where this block sits, which is what a drop reorders against. */
  at: number;
  on: {
    change: (text: string) => void;
    remove: () => void;
    keep: (tool: string) => void;
    /** `/` typed where a menu is wanted. */
    slash: () => void;
    /** Moving this block, which is moving it in the instruction. */
    drag: {
      onStart: (index: number) => void;
      onOver: (index: number) => void;
      onDrop: () => void;
    };
  };
  tools: { catalogue: Tool[]; policies: Policy[] };
  findings: Finding[];
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
    on.change(next);
    if (next.endsWith("@")) setCiting(true);
    // `/` at the start of a line, which is where somebody reaches for a menu
    // rather than a slash. Mid-sentence it is a slash and stays one.
    if (next.endsWith("/") && /(^|\n)\/$/.test(next)) on.slash();
  };

  const cite = (tool: string) => {
    on.change(`${block.text.replace(/@$/, "")}${tool}`);
    setCiting(false);
  };

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        on.drag.onOver(at);
      }}
      onDrop={(e) => {
        e.preventDefault();
        on.drag.onDrop();
      }}
      className="group grid grid-cols-[104px_minmax(0,68ch)_auto] items-start gap-x-5 rounded-md py-2.5 transition-colors hover:bg-surface-hover"
    >
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
      <BlockText
        block={block}
        writing={writing}
        onWriting={setWriting}
        tools={tools}
        typed={typed}
        cite={{ open: citing, onPick: cite, onClose: () => setCiting(false) }}
      />

      {/* Outside the swap between writing and reading, because it belongs to
          the block rather than to a way of looking at it — and because a
          button inside the read pane disappears on the focus that clicking it
          causes, so the click lands on nothing. */}
      {findings.length > 0 && (
        <div className="col-start-2">
          {findings.map((finding) => (
            <InstructionFinding
              key={finding.tool}
              finding={finding}
              onRemove={() =>
                on.change(withoutSentence(block.text, finding.tool))
              }
              onKeep={() => on.keep(finding.tool)}
            />
          ))}
        </div>
      )}

      <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100">
        {/* The handle rather than the row: a paragraph you cannot select
            with the pointer is worse than one you have to grab by its grip. */}
        <span
          draggable
          onDragStart={() => on.drag.onStart(at)}
          onDragEnd={on.drag.onDrop}
          aria-label={t("agents.moveBlock")}
          className="cursor-grab text-text-disabled"
        >
          <GripVertical className="size-4" aria-hidden />
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7"
          onClick={on.remove}
          aria-label={t("agents.removeBlock")}
        >
          <Trash2 className="size-3.5" aria-hidden />
        </Button>
      </div>
    </div>
  );
}

