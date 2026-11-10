import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { TableCell, TableRow } from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import { ExperimentConfigText } from "@/features/experiments/experiment-config";
import { formatInstant } from "@/lib/format";
import type { ExperimentListing } from "@/features/experiments/experiments-api";

/** One experiment, by what it pinned. What it found is one click away. */
export function ExperimentListRow({
  experiment,
  picked,
  onToggle,
  onOpen,
}: {
  experiment: ExperimentListing;
  picked: boolean;
  onToggle: () => void;
  onOpen: () => void;
}) {
  const { t } = useTranslation();
  const opened = formatInstant(experiment.openedAt);

  return (
    <TableRow data-state={picked ? "selected" : undefined}>
      <TableCell>
        <Checkbox
          checked={picked}
          onCheckedChange={onToggle}
          aria-label={t("experiments.pickOne", { opened })}
        />
      </TableCell>
      <TableCell>
        <Button variant="link" size="sm" className="h-auto p-0" onClick={onOpen}>
          {opened}
        </Button>
      </TableCell>
      <TableCell>
        <Mono>{experiment.datasetVersion}</Mono>
      </TableCell>
      <TableCell>
        <Mono>{experiment.version.slice(0, 9)}</Mono>
      </TableCell>
      <TableCell>
        <ExperimentConfigText config={experiment.config} />
      </TableCell>
      <TableCell className="tabular-nums">{experiment.repetitions}</TableCell>
      <TableCell className="tabular-nums">
        {t("experiments.runsOverCases", { runs: experiment.runs, count: experiment.cases })}
      </TableCell>
    </TableRow>
  );
}
