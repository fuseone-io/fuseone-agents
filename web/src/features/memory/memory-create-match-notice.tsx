import { AlertCircle, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import type { useMemoryCreateMatch } from "@/features/memory/memory-create-match";
import { MemoryDuplicateNotice } from "@/features/memory/memory-duplicate-notice";

export function MemoryCreateMatchNotice({
  state,
  reason,
  onImproveShared,
}: {
  state: ReturnType<typeof useMemoryCreateMatch>;
  reason: string;
  onImproveShared: () => void;
}) {
  const { t } = useTranslation();
  if (state.loading) {
    return (
      <div
        role="status"
        aria-label={t("memory.matchChecking")}
        className="grid gap-2"
      >
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-12 w-full" />
      </div>
    );
  }
  if (state.issue) {
    const copy = {
      run: {
        title: "memory.evidenceRunCheckFailed",
        hint: "memory.evidenceRunCheckFailedHint",
      },
      agent: {
        title: "memory.evidenceRunAgentMissing",
        hint: "memory.evidenceRunAgentMissingHint",
      },
      match: {
        title: "memory.matchCheckFailed",
        hint: "memory.matchCheckFailedHint",
      },
    }[state.issue.kind];
    return (
      <Alert variant={state.issue.kind === "match" ? "default" : "destructive"}>
        <AlertCircle aria-hidden />
        <AlertTitle>{t(copy.title)}</AlertTitle>
        <AlertDescription className="grid gap-2">
          <span>{t(copy.hint)}</span>
          {"retry" in state.issue ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={state.issue.retry}
            >
              <RefreshCw aria-hidden />
              {t("common.retry")}
            </Button>
          ) : null}
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <MemoryDuplicateNotice
      match={state.data}
      reason={reason}
      onImproveShared={onImproveShared}
    />
  );
}
