import { useTranslation } from "react-i18next";
import { WorkItemRefLink } from "@/features/work-items/work-item-ref-link";
import type { Step } from "@/lib/api/client";

type Inheritance = { ref: string; source: string; fromRun: string };

/**
 * The work item a run's start inherited (#204), or null: the card the event
 * or the GitHub delivery that opened the run was about. A payload the console
 * does not recognise inherits nothing.
 */
function inheritanceOf(step: Step): Inheritance | null {
  if (step.kind !== "run_started") return null;
  const item = (step.payload as Record<string, unknown> | undefined)?.work_item;
  if (!item || typeof item !== "object") return null;
  const { ref, source, from_run: fromRun } = item as Record<string, unknown>;
  if (typeof ref !== "string" || !ref) return null;
  if (source === "event" && typeof fromRun === "string" && fromRun) {
    return { ref, source, fromRun };
  }
  if (source === "github_delivery") return { ref, source, fromRun: "" };
  return null;
}

/**
 * On a run's opening line, the card it inherited and where from, leading to
 * that card's trail. Attribution only: the run holds nothing by it.
 */
export function StartInheritance({ step }: { step: Step }) {
  const { t } = useTranslation();
  const inherited = inheritanceOf(step);
  if (!inherited) return null;
  const label =
    inherited.source === "event"
      ? t("runs.storyInheritedFromEvent", {
          ref: inherited.ref,
          run: inherited.fromRun,
        })
      : t("runs.storyInheritedFromDelivery", { ref: inherited.ref });
  return (
    <p className="mt-0.5 text-xs text-muted-foreground">
      <WorkItemRefLink reference={inherited.ref} label={label} />
    </p>
  );
}
