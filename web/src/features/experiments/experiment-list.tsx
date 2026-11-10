import { useState } from "react";
import { useTranslation } from "react-i18next";
import { GitCompareArrows, TestTubes } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Panel } from "@/components/shared/panel";
import { ExperimentListRow } from "@/features/experiments/experiment-list-row";
import { LoadFailed } from "@/features/experiments/load-failed";
import { useExperiments } from "@/features/experiments/experiments-api";
import { inOpeningOrder } from "@/features/experiments/experiment-order";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

/**
 * An agent's newest experiments, and the two to compare.
 *
 * Compared oldest to newest whatever order they were picked in, so the same two
 * always read the same way round: "was" is the one that opened first.
 */
export function ExperimentList({
  agentId,
  onOpen,
  onCompare,
}: {
  agentId: string;
  onOpen: (id: string) => void;
  onCompare: (from: string, to: string) => void;
}) {
  const { t } = useTranslation();
  const list = useExperiments(agentId);
  const [picked, setPicked] = useState<string[]>([]);

  if (list.isLoading) return <Skeleton className="h-48 w-full rounded-xl" />;
  if (list.error || !list.data) {
    return (
      <LoadFailed message={t("experiments.listFailed")} onRetry={() => void list.refetch()} />
    );
  }
  const experiments = list.data.experiments;
  if (experiments.length === 0) {
    return (
      <Panel>
        <div className="flex items-start gap-3 text-sm text-muted-foreground">
          <TestTubes className="mt-0.5 size-4 shrink-0" aria-hidden />
          <p>{t("experiments.empty")}</p>
        </div>
      </Panel>
    );
  }

  const toggle = (id: string) =>
    setPicked((now) =>
      now.includes(id) ? now.filter((p) => p !== id) : [...now, id].slice(-2),
    );
  const compare = () => {
    const [a, b] = picked;
    if (a === undefined || b === undefined) return;
    const [from, to] = inOpeningOrder(experiments, a, b);
    onCompare(from, to);
  };

  return (
    <Panel
      title={t("experiments.listTitle")}
      action={
        <Button variant="outline" size="sm" disabled={picked.length !== 2} onClick={compare}>
          <GitCompareArrows className="size-4" aria-hidden />
          {t("experiments.compare")}
        </Button>
      }
      flush
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">
              <span className="sr-only">{t("experiments.pick")}</span>
            </TableHead>
            <TableHead>{t("experiments.opened")}</TableHead>
            <TableHead>{t("experiments.datasetVersion")}</TableHead>
            <TableHead>{t("experiments.agentVersion")}</TableHead>
            <TableHead>{t("experiments.configuration")}</TableHead>
            <TableHead>{t("experiments.repetitions")}</TableHead>
            <TableHead>{t("experiments.runs")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {experiments.map((e) => (
            <ExperimentListRow
              key={e.id}
              experiment={e}
              picked={picked.includes(e.id)}
              onToggle={() => toggle(e.id)}
              onOpen={() => onOpen(e.id)}
            />
          ))}
        </TableBody>
      </Table>
    </Panel>
  );
}

