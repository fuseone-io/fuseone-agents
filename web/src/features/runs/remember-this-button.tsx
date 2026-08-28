import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Brain } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { useMe } from "@/features/session/api";
import { useRun } from "@/features/runs/api";
import { citableAsEvidence } from "@/features/runs/run-citations";
import { RememberThisForm } from "@/features/runs/remember-this-form";
import type { Step } from "@/lib/api/client";

/**
 * Teaching a memory from where the fact was learned.
 *
 * On the step rather than on the page, because a memory cites one output of one
 * run and the person deciding is looking at it. The memory page still exists and
 * still takes a run id typed by hand; this is the path that does not ask anybody
 * to copy an identifier between two screens.
 *
 * Hidden without permission to publish, which is a courtesy and not a control —
 * the server checks again, in the scope of the run itself.
 */
export function RememberThisButton({
  runId,
  step,
}: {
  runId: string;
  step: Step;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { data: me } = useMe();
  const run = useRun(runId);

  if (!citableAsEvidence(step)) return null;
  if (me && !me.can.includes("agent:publish")) return null;

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => setOpen(true)}
      >
        <Brain className="size-4" aria-hidden />
        {t("memory.rememberThis")}
      </Button>
      <SheetContent className="w-full overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{t("memory.rememberThis")}</SheetTitle>
          <SheetDescription>{t("memory.rememberThisHint")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-6">
          {run.data ? (
            <RememberThisForm
              runId={runId}
              step={step}
              run={run.data}
              onDone={() => setOpen(false)}
            />
          ) : run.error ? (
            <p role="alert" className="text-sm text-danger">
              {t("memory.rememberThisUnavailable")}
            </p>
          ) : (
            <Skeleton className="h-64 w-full" />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
