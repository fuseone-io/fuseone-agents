import { useTranslation } from "react-i18next";
import { Plus, UserRoundCheck } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { cn } from "@/lib/utils";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * The sequence as a strip of cards.
 *
 * A row that scrolls sideways rather than a graph that lays itself out. The
 * specification is a sequence with no branch, so there is nothing for a layout
 * engine to decide: cards in order, a rule between them, and the whole thing
 * is deterministic because it is one line — the same version draws the same
 * picture with nothing to compute (FU-17, FU-18).
 *
 * Selecting is what a card does. Reordering lives on the grip in the text
 * view, where a list is the natural thing to drag inside of.
 */
export function StepStrip({
  steps,
  selected,
  stops,
  onSelect,
  onAdd,
}: {
  steps: AgentStep[];
  selected?: number;
  /** Which stages the Gate will not simply allow. */
  stops: (at: number) => boolean;
  onSelect?: (at: number) => void;
  /** Absent where the sequence is read rather than written. */
  onAdd?: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div
      className="min-w-0 flex-1 overflow-auto bg-[radial-gradient(var(--border)_1px,transparent_0)] bg-[length:16px_16px] px-6 py-7"
      style={{ backgroundPosition: "-1px -1px" }}
    >
      <div className="flex min-w-min items-center">
        {steps.map((step, at) => (
          <div key={at} className="flex items-center">
            {at > 0 && <span className="h-px w-7 shrink-0 bg-border-strong" />}
            <button
              type="button"
              onClick={() => onSelect?.(at)}
              className={cn(
                "flex w-[212px] shrink-0 flex-col gap-2 rounded-lg border bg-card p-3 text-left transition-colors",
                at === selected
                  ? "border-primary shadow-md"
                  : "border-border shadow-xs hover:border-border-strong",
              )}
            >
              <div className="flex items-center gap-2">
                <span
                  className={cn(
                    "grid size-5 place-items-center rounded-pill font-mono text-[11px] tabular-nums",
                    at === selected
                      ? "bg-primary text-primary-foreground"
                      : "border border-border bg-muted text-muted-foreground",
                  )}
                >
                  {at + 1}
                </span>
                {at === selected ? (
                  <span className="text-2xs text-text-accent">
                    {t("agents.selectedStep")}
                  </span>
                ) : (
                  stops(at) && (
                    <span className="flex items-center gap-1.5 text-2xs text-warning">
                      <UserRoundCheck className="size-3.5" aria-hidden />
                      {t("agents.mayStopHere")}
                    </span>
                  )
                )}
              </div>

              <span className="text-sm text-pretty">
                {step.name || (
                  <span className="italic text-muted-foreground">
                    {t("agents.unnamedStep")}
                  </span>
                )}
              </span>
              <Mono dim className="truncate text-[11px]">
                {(step.reaches ?? []).join(", ") || t("agents.thinksOnly")}
              </Mono>
            </button>
          </div>
        ))}

        {onAdd && (
          <>
            <span className="h-px w-7 shrink-0 bg-border-strong" />
            <button
              type="button"
              onClick={onAdd}
              aria-label={t("agents.addStep")}
              className="grid size-11 shrink-0 place-items-center rounded-lg border border-dashed border-border text-muted-foreground transition-colors hover:border-border-strong hover:text-foreground"
            >
              <Plus className="size-4" aria-hidden />
            </button>
          </>
        )}
      </div>
    </div>
  );
}
