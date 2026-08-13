import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { AlertCircle, Inbox, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

// Every view that loads data owes the reader four states. These are the three
// that are easy to skip, so they live here and are cheap to reach for.

export function LoadingRows({ rows = 5 }: { rows?: number }) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-2" aria-busy="true" aria-live="polite">
      <span className="sr-only">{t("common.loading")}</span>
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-12 w-full" />
      ))}
    </div>
  );
}

export function EmptyState({
  title,
  hint,
  icon,
}: {
  title: string;
  hint: string;
  icon?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed px-6 py-12 text-center">
      <div className="text-muted-foreground">
        {icon ?? <Inbox className="size-6" />}
      </div>
      <p className="font-medium">{title}</p>
      {/* An empty state that only says "nothing here" wastes the moment: say
          what would appear and what produces it. */}
      <p className="max-w-md text-sm text-muted-foreground">{hint}</p>
    </div>
  );
}

export function ErrorState({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  const problem = error instanceof ApiError ? error : undefined;
  // The words are this console's; the particulars are the server's. Showing
  // the server's own sentence meant showing Portuguese to an English reader
  // for half the refusals and English to a Portuguese one for the other half.
  const message = problemMessage(error, t);
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-3 rounded-lg border border-destructive/40 bg-destructive/5 px-6 py-5"
    >
      <div className="flex items-center gap-2 font-medium text-destructive">
        <AlertCircle className="size-4" />
        {message}
      </div>
      <p className="text-sm text-muted-foreground">
        {problem?.detail ?? t("common.loadFailedHint")}
      </p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw className="size-4" />
          {t("common.retry")}
        </Button>
      )}
    </div>
  );
}
