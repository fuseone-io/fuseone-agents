import { useTranslation } from "react-i18next";
import { AlertTriangle, Undo2 } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import type { CompensationAct } from "@/features/runs/compensation-api";

/**
 * What abandoning this run would undo, in the order it would happen.
 *
 * Reverse order, because that is the only one that makes sense: undoing the
 * order before the charge it paid for leaves a refund against nothing. An act
 * nothing can undo is shown with the rest and marked, never omitted — it is
 * the line somebody has to act on by hand.
 */
export function CompensationPlan({
  acts,
  loading,
}: {
  acts: CompensationAct[] | undefined;
  loading: boolean;
}) {
  const { t } = useTranslation();

  if (loading) {
    return (
      <div className="flex flex-col gap-2">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-4/5" />
      </div>
    );
  }
  if (!acts || acts.length === 0) {
    return (
      <p className="rounded-lg border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
        {t("compensation.nothingStanding")}
      </p>
    );
  }

  return (
    <ul className="flex flex-col gap-1.5">
      {acts.map((act) => (
        <ActRow key={`${act.tool}-${act.seq}`} act={act} />
      ))}
    </ul>
  );
}

function ActRow({ act }: { act: CompensationAct }) {
  const { t } = useTranslation();
  const standing = !act.undo;

  return (
    <li className="flex items-center gap-2 rounded-lg border px-3 py-2 text-xs">
      {standing ? (
        <AlertTriangle aria-hidden className="size-3.5 shrink-0 text-warning" />
      ) : (
        <Undo2 aria-hidden className="size-3.5 shrink-0 text-muted-foreground" />
      )}
      <Mono className="text-2xs text-muted-foreground">
        {t("compensation.atStep", { seq: act.seq })}
      </Mono>
      <Mono className="text-xs">{act.tool}</Mono>
      {standing ? (
        <span className="ml-auto text-warning">
          {t("compensation.nothingUndoesIt")}
        </span>
      ) : (
        <span className="ml-auto flex items-center gap-1.5 text-muted-foreground">
          {t("compensation.undoneBy")}
          <Mono className="text-xs text-foreground">{act.undo}</Mono>
        </span>
      )}
    </li>
  );
}
