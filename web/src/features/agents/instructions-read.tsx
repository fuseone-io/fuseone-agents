import { useTranslation } from "react-i18next";
import { labelOf, parse } from "@/features/agents/instruction-blocks";
import { cn } from "@/lib/utils";

/**
 * A published instruction, read the way it was written.
 *
 * The same margin the editor gives it: a well-written prompt already has a
 * purpose, a way to act and a point where it stops, and a version rendered as
 * one paragraph throws that away at exactly the moment somebody is trying to
 * work out what the agent was told.
 *
 * Structure without controls. A version is changed by publishing another one,
 * never by editing one that runs already reference — so the label is a word
 * here and a menu there, and nothing on this screen can be pressed.
 *
 * An unlabelled block gets an empty margin rather than the word the editor
 * shows. There the label names the option to give it one; here it would be a
 * word that means nothing to whoever is reading.
 */
export function InstructionsRead({ instructions }: { instructions: string }) {
  const { i18n } = useTranslation();
  const blocks = parse(instructions);
  // An instruction nobody labelled — which most instructions ever written are
  // — gets no column at all. A gutter reserved and left empty on every row
  // reads as a defect rather than as an absence.
  const labelled = blocks.some((block) => block.kind !== "prose");

  return (
    <div className="min-w-0 flex flex-col gap-0.5">
      {blocks.map((block, at) => (
        <div
          key={at}
          className={cn(
            "min-w-0 items-start py-2.5",
            labelled && "grid grid-cols-[72px_minmax(0,1fr)] gap-x-3 sm:grid-cols-[104px_minmax(0,1fr)] sm:gap-x-5",
            !labelled && "max-w-[68ch]",
          )}
        >
          {labelled && (
            <span
              className={cn(
                "pt-[3px] text-right text-[10px]/5 font-medium uppercase tracking-label",
                block.kind === "never" ? "text-danger" : "text-muted-foreground",
              )}
            >
              {labelOf(block.kind, i18n.language)}
            </span>
          )}
          <p className="min-w-0 text-base/[1.65] whitespace-pre-wrap break-words text-pretty">
            {block.text}
          </p>
        </div>
      ))}
    </div>
  );
}
