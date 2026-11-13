import { useTranslation } from "react-i18next";
import { ErrorState } from "@/components/shared/states";
import { ApiError } from "@/lib/api/client";

/**
 * A refusal to answer for the agent's target scope.
 *
 * A 404 is the server declining to confirm a scope the reader cannot see, not
 * a missing page: it gets its own sentence, so the author learns that the
 * area is out of their reach rather than that something broke. Anything else
 * is an ordinary failure, with a retry.
 */
export function TargetError({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  if (!(error instanceof ApiError) || error.status !== 404) {
    return <ErrorState error={error} onRetry={onRetry} />;
  }
  return (
    <div
      role="alert"
      className="flex flex-col gap-1 rounded-lg border border-destructive/40 bg-destructive/5 px-4 py-3"
    >
      <p className="text-sm font-medium text-destructive">
        {t("agents.targetUnreadable")}
      </p>
      <p className="text-xs text-muted-foreground">
        {t("agents.targetUnreadableHint")}
      </p>
    </div>
  );
}
