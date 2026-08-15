import { useTranslation } from "react-i18next";
import { labelOf } from "@/features/agents/instruction-blocks";
import { diffInstructions, type BlockDiff } from "@/features/agents/instruction-diff";
import { cn } from "@/lib/utils";

/**
 * What publishing would change in the prose.
 *
 * The publish summary can only say that the instruction changed; it is the one
 * entry there nobody can check. A character count does not distinguish a
 * sentence tightened from a rule deleted, and telling those apart is what a
 * reviewer is for.
 *
 * Laid out on the same grid as the editor — label in the margin, prose in the
 * column — so the two are read the same way. Marked with `ins` and `del`
 * rather than with colour alone: an underline and a strike-through are the
 * meaning, and everything else is emphasis.
 */
export function InstructionsDiffView({
  was,
  now,
}: {
  /** The instruction as published. */
  was: string;
  now: string;
}) {
  const { t } = useTranslation();
  const diff = diffInstructions(was, now);

  if (diff.every((block) => block.state === "same")) {
    return (
      <p className="py-6 text-center text-xs text-muted-foreground">
        {t("agents.proseUnchanged")}
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-0.5">
      {diff.map((block, at) => (
        <DiffRow key={at} block={block} />
      ))}
    </div>
  );
}

function DiffRow({ block }: { block: BlockDiff }) {
  const { t, i18n } = useTranslation();
  const label = labelOf(block.kind, i18n.language) || t("agents.blockProse");

  return (
    <div className="grid grid-cols-[104px_minmax(0,68ch)] items-start gap-x-5 rounded-md py-2.5">
      <span
        className={cn(
          "pt-[3px] text-right text-[10px]/5 font-medium uppercase tracking-label",
          block.state === "removed" ? "line-through" : "",
          block.kind === "never" ? "text-danger" : "text-muted-foreground",
        )}
      >
        {label}
      </span>

      <p className="text-base/[1.65] whitespace-pre-wrap text-pretty">
        {block.pieces.map((piece, at) => {
          if (piece.kind === "added") {
            return (
              <ins
                key={at}
                className="bg-success-surface text-success no-underline [text-decoration-line:underline] [text-underline-offset:3px]"
              >
                {piece.text}
              </ins>
            );
          }
          if (piece.kind === "removed") {
            return (
              <del key={at} className="bg-danger-surface text-danger">
                {piece.text}
              </del>
            );
          }
          return <span key={at}>{piece.text}</span>;
        })}
      </p>
    </div>
  );
}
