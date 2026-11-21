import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { LoadFailed } from "@/features/experiments/load-failed";
import { byVersion } from "@/features/workbench/candidate-versions";
import { useCandidates } from "@/features/workbench/candidates-api";
import { formatInstant } from "@/lib/format";
import { problemMessage } from "@/lib/api/problem-message";

/** The candidates the person may see, newest first, each to open on its sealed text. */
export function CandidatesList({ agentId, onOpen }: { agentId: string; onOpen: (version: string) => void }) {
  const { t } = useTranslation();
  const candidates = useCandidates(agentId);

  if (candidates.isLoading) {
    return (
      <div role="status" aria-label={t("workbench.candidates.reading")} className="flex flex-col gap-2">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
      </div>
    );
  }
  if (candidates.error) {
    return <LoadFailed message={problemMessage(candidates.error, t)} onRetry={() => void candidates.refetch()} />;
  }
  if (!candidates.data?.length) {
    return (
      <div className="flex flex-col gap-1 rounded-lg border border-dashed p-4">
        <p className="text-sm font-medium">{t("workbench.candidates.empty")}</p>
        <p className="text-sm text-muted-foreground">{t("workbench.candidates.emptyBody")}</p>
      </div>
    );
  }
  // The version opens the candidate, so no column of actions has to fit; who sealed it
  // steps aside on a phone. A version sealed again shows once, under its newest seal.
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("workbench.candidates.version")}</TableHead>
          <TableHead>{t("workbench.candidates.sealedAt")}</TableHead>
          <TableHead className="hidden sm:table-cell">{t("workbench.candidates.sealedByColumn")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {byVersion(candidates.data).map(({ seal: c, seals }) => (
          <TableRow key={c.version}>
            <TableCell>
              <Button size="sm" variant="link" className="h-auto p-0" onClick={() => onOpen(c.version)} aria-label={t("workbench.candidates.open", { version: c.version })}>
                <Mono>{c.version}</Mono>
              </Button>
            </TableCell>
            <TableCell>
              <Mono dim>{formatInstant(c.sealedAt)}</Mono>
              {seals > 1 && <Badge variant="secondary" className="ml-2">{t("workbench.candidates.seals", { count: seals })}</Badge>}
            </TableCell>
            <TableCell className="hidden sm:table-cell">{c.sealedBy}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
