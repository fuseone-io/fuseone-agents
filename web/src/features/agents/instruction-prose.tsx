import { ShieldX, Wrench } from "lucide-react";
import { segments } from "@/features/agents/instruction-entities";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * The block, read: prose in sans with what it names picked out.
 *
 * Chips rather than words, so the platform knows what the text is talking
 * about and can say when a sentence promises what a policy already refuses.
 * They are a rendering only — the payload carries the bare identifier — so
 * nothing here changes what the model receives.
 *
 * A denied tool gets the wavy underline as well as the colour: this is the one
 * mark on the screen that says "this sentence is not true", and colour alone
 * is not something everybody can see.
 */
export function InstructionProse({
  text,
  catalogue,
  policies,
}: {
  text: string;
  catalogue: Tool[];
  policies: Policy[];
}) {
  return (
    <p className="min-w-0 text-base/[1.65] whitespace-pre-wrap break-words text-pretty">
      {segments(text, catalogue, policies).map((segment, at) => {
        if (segment.kind === "text") return <span key={at}>{segment.text}</span>;

        if (segment.kind === "limit") {
          return (
            <span
              key={at}
              className="rounded-sm bg-warning-surface px-1 font-mono text-xs break-all text-warning"
            >
              {segment.text}
            </span>
          );
        }

        const denied = segment.kind === "denied";
        return (
          <span
            key={at}
            className={
              denied
                ? "inline-flex max-w-full flex-wrap items-baseline gap-1 rounded-sm bg-danger-surface px-1.5 font-mono text-xs break-all text-danger [text-decoration-line:underline] [text-decoration-style:wavy] [text-underline-offset:3px]"
                : "inline-flex max-w-full flex-wrap items-baseline gap-1 rounded-sm border border-border bg-muted px-1.5 font-mono text-xs break-all text-text-secondary"
            }
          >
            {denied ? (
              <ShieldX className="size-[11px] self-center" aria-hidden />
            ) : (
              <Wrench className="size-[11px] self-center" aria-hidden />
            )}
            {segment.text}
          </span>
        );
      })}
    </p>
  );
}
