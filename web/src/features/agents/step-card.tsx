import { useTranslation } from "react-i18next";
import { GripVertical, Pencil, UserRoundCheck, Wrench } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * One stage, read rather than filled in.
 *
 * The sentence is text and not a field: a list of open inputs is a form, and
 * a process somebody wants to read back before publishing should read like a
 * process. Editing is a deliberate act, on the pencil.
 *
 * The tools are what this stage may reach, and the amber line is what the
 * Gate will do about it — stated, never set here. The verdict belongs to the
 * policy that produced it.
 */
export function StepCard({
  step,
  index,
  stops,
  onEdit,
  drag,
}: {
  step: AgentStep;
  index: number;
  /** The Gate's answer, when it is not simply allowed. */
  stops?: string;
  onEdit: () => void;
  /** Moving this card, which is moving it in the sequence. */
  drag: {
    onStart: (at: number) => void;
    onOver: (at: number) => void;
    onDrop: () => void;
  };
}) {
  const { t } = useTranslation();
  const reaches = step.reaches ?? [];

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        drag.onOver(index);
      }}
      onDrop={(e) => {
        e.preventDefault();
        drag.onDrop();
      }}
      className="group flex items-start gap-3 rounded-lg border border-border bg-card px-3.5 py-3 shadow-xs transition-colors hover:border-border-strong"
    >
      {/* The handle rather than the card: a row full of text you cannot
          select is worse than one you have to grab by its grip. */}
      <span
        draggable
        onDragStart={() => drag.onStart(index)}
        onDragEnd={drag.onDrop}
        aria-label={t("agents.moveStep", { number: index + 1 })}
        className="mt-0.5 shrink-0 cursor-grab text-text-disabled"
      >
        <GripVertical className="size-4" aria-hidden />
      </span>
      <span className="grid size-[22px] shrink-0 place-items-center rounded-pill border border-border bg-muted font-mono text-[11px] tabular-nums text-muted-foreground">
        {index + 1}
      </span>

      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <p className="text-sm text-pretty">
          {step.name || (
            <span className="italic text-muted-foreground">
              {t("agents.unnamedStep")}
            </span>
          )}
        </p>

        <div className="flex flex-wrap items-center gap-1.5">
          {reaches.length === 0 ? (
            <span className="text-2xs text-muted-foreground">
              {t("agents.thinksOnly")}
            </span>
          ) : (
            reaches.map((tool) => (
              <span
                key={tool}
                className="flex h-5 items-center gap-1.5 rounded-md border border-border bg-muted px-1.5 text-text-secondary"
              >
                <Wrench className="size-[11px]" aria-hidden />
                <Mono className="text-[11px]">{tool}</Mono>
              </span>
            ))
          )}
          {stops && (
            <span className="flex items-center gap-1.5 text-2xs text-warning">
              <UserRoundCheck className="size-3.5" aria-hidden />
              {stops}
            </span>
          )}
        </div>
      </div>

      {/* Reached on hover and on focus, so it is not a control that only
          exists for a mouse. */}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
        onClick={onEdit}
        aria-label={t("agents.editStep", { number: index + 1 })}
      >
        <Pencil className="size-3.5" aria-hidden />
      </Button>
    </div>
  );
}

