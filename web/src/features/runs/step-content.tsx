import { useTranslation } from "react-i18next";
import { Skeleton } from "@/components/ui/skeleton";
import { useStepContent } from "@/features/runs/api";
import { JsonBody } from "@/components/shared/json-body";
import { RunOutcomeBody } from "@/features/runs/run-outcome-body";

/**
 * What a step referenced, fetched when somebody opens it.
 *
 * Never with the trail: the trail is read constantly and by many people, and
 * arguments and results routinely carry personal data. Opening one step is a
 * deliberate act; loading all of them would not be.
 */
export function StepContent({
  runId,
  seq,
  open,
  prose = false,
}: {
  runId: string;
  seq: number;
  open: boolean;
  /** A closing answer is a document the model wrote; everything else the trail
   *  references is a payload. They are read differently and rendered so. */
  prose?: boolean;
}) {
  const { t } = useTranslation();
  const content = useStepContent(runId, seq, open);

  if (content.isLoading)
    return <Skeleton className="mt-2.5 h-16 w-full rounded-lg" />;
  if (content.error || !content.data) {
    return (
      <p className="mt-2.5 rounded-lg border border-border bg-muted p-3 text-xs text-muted-foreground">
        {t("runs.contentGone")}
      </p>
    );
  }

  if (prose) {
    return (
      <div className="mt-2.5 max-h-[min(60vh,32rem)] overflow-auto rounded-lg border border-border bg-muted px-3 py-1">
        <RunOutcomeBody body={content.data.content} />
      </div>
    );
  }
  return <JsonBody body={content.data.content} className="mt-2.5" />;
}

