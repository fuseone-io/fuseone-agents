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

  return (
    <div className="flex flex-col gap-0.5">
      {parse(instructions).map((block, at) => (
        <div
          key={at}
          className="grid grid-cols-[104px_minmax(0,68ch)] items-start gap-x-5 py-2.5"
        >
          <span
            className={cn(
              "pt-[3px] text-right text-[10px]/5 font-medium uppercase tracking-label",
              block.kind === "never" ? "text-danger" : "text-muted-foreground",
            )}
          >
            {labelOf(block.kind, i18n.language)}
          </span>
          <p className="text-base/[1.65] whitespace-pre-wrap text-pretty">
            {block.text}
          </p>
        </div>
      ))}
    </div>
  );
}
