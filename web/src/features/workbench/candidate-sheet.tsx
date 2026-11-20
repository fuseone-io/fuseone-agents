import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { TestTubes } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { PropertiesSheet, PropertiesSheetBody, PropertiesSheetFooter } from "@/components/shared/properties-sheet";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useCandidate } from "@/features/workbench/candidates-api";
import { formatInstant } from "@/lib/format";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * One candidate, on the text it was sealed with: read from the candidate, byte
 * for byte, never from the draft, which may have moved on since.
 */
export function CandidateSheet({
  agentId,
  version,
  onClose,
}: {
  agentId: string;
  version: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const candidate = useCandidate(agentId, version);

  return (
    <PropertiesSheet
      open={version !== ""}
      onOpenChange={(open) => !open && onClose()}
      title={<Mono>{version}</Mono>}
      description={t("workbench.candidates.sheetDescription")}
    >
      <PropertiesSheetBody className="flex flex-col gap-4">
        {candidate.isLoading && (
          <div role="status" aria-label={t("workbench.candidates.readingOne")} className="flex flex-col gap-2">
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-40 w-full" />
          </div>
        )}
        {candidate.error && (
          <LoadFailed message={problemMessage(candidate.error, t)} onRetry={() => void candidate.refetch()} />
        )}
        {candidate.data && (
          <>
            <p className="text-xs text-muted-foreground">
              {t("workbench.candidates.sealedBy", { by: candidate.data.sealedBy, at: formatInstant(candidate.data.sealedAt) })}
            </p>
            <Mono className="whitespace-pre-wrap wrap-anywhere rounded-md border bg-muted p-3 text-xs">
              {candidate.data.definition}
            </Mono>
          </>
        )}
      </PropertiesSheetBody>
      {candidate.data && (
        <PropertiesSheetFooter>
          <Button asChild variant="outline">
            <Link to={experimentOn(agentId, candidate.data.sealId)}>
              <TestTubes className="size-4" aria-hidden />
              {t("workbench.candidates.experiment")}
            </Link>
          </Button>
        </PropertiesSheetFooter>
      )}
    </PropertiesSheet>
  );
}

/** The experiments screen, its form open on this seal: the seal the sheet read, by id. */
function experimentOn(agentId: string, sealId: string): string {
  const query = new URLSearchParams({ new: "1", seal: sealId });
  return `/agents/${encodeURIComponent(agentId)}/experiments?${query.toString()}`;
}
