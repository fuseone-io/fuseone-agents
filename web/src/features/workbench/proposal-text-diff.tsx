import { align } from "@/features/agents/align";
import { cn } from "@/lib/utils";

/**
 * The instructions as written now against what a block proposal would leave,
 * line by line and byte for byte.
 *
 * Not the publish summary's block diff: that one matches blocks by what they
 * say and reads every label in the reader's language, which is right for
 * reviewing prose and wrong here. A proposal can move a block without
 * changing a word, or rewrite its label in another language, and both are
 * what the model will read. Lines keep their place, so a move shows where a
 * block leaves and where it lands, and a label shows as it will be written.
 * Marked with `ins` and `del`; colour only repeats it.
 */
export function ProposalTextDiff({ was, now }: { was: string; now: string }) {
  const before = was.split("\n");
  const after = now.split("\n");
  return (
    <div className="max-h-[min(48vh,28rem)] overflow-auto rounded-lg border border-border bg-muted p-3 text-sm">
      {align(before, after).map((step, at) => {
        const text = step.kind === "added" ? after[step.bi!] : before[step.ai!];
        const line = cn("block min-h-5 whitespace-pre-wrap wrap-anywhere no-underline");
        if (step.kind === "added") {
          return <ins key={at} className={cn(line, "bg-success-surface text-success")}>{text}</ins>;
        }
        if (step.kind === "removed") {
          return <del key={at} className={cn(line, "bg-danger-surface text-danger")}>{text}</del>;
        }
        return <span key={at} className={line}>{text}</span>;
      })}
    </div>
  );
}
