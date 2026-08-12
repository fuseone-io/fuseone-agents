import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { stateOfPhase } from "@/lib/agent-state";
import { PHASE_LABELS } from "@/features/runs/phase-badge";
import { formatCost, formatDuration, formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Run } from "@/lib/api/client";

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";
const NUM = "text-right font-mono text-xs tabular-nums";

/**
 * The executions table.
 *
 * Everything the platform produced reads in mono with fixed-width digits and
 * sits on the right, so the four numeric columns can be compared down the page
 * without reading each cell.
 */
export function RunsTable({
  runs,
  selected,
  onSelect,
}: {
  runs: Run[];
  /** The run whose trace is open beside the table, if any. */
  selected?: string;
  /**
   * Opens the run beside the table instead of navigating to it. Without this
   * a row goes to the run's own screen, which is right on a page that is only
   * the table and wrong on one somebody is scanning.
   */
  onSelect?: (runId: string) => void;
}) {
  const navigate = useNavigate();
  const { t } = useTranslation();

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className={HEAD}>{t("runs.columnRun")}</TableHead>
          <TableHead className={HEAD}>{t("runs.columnAgent")}</TableHead>
          <TableHead className={HEAD}>{t("runs.columnState")}</TableHead>
          <TableHead className={`${HEAD} text-right`}>
            {t("runs.columnSteps")}
          </TableHead>
          <TableHead className={`${HEAD} text-right`}>
            {t("runs.columnDuration")}
          </TableHead>
          <TableHead className={`${HEAD} text-right`}>
            {t("runs.columnCost")}
          </TableHead>
          <TableHead className={`${HEAD} text-right`}>
            {t("runs.columnStarted")}
          </TableHead>
        </TableRow>
      </TableHeader>

      <TableBody>
        {runs.map((run) => {
          const state = stateOfPhase(run.phase);
          return (
            <TableRow
              key={run.runId}
              onClick={() =>
                onSelect ? onSelect(run.runId) : navigate(`/runs/${run.runId}`)
              }
              // The selected row keeps the accent bar the design gives it, so
              // the trace beside the table is visibly about this row.
              data-state={selected === run.runId ? "selected" : undefined}
              className={cn(
                "h-10 cursor-pointer border-border-subtle",
                selected === run.runId &&
                  "bg-surface-accent shadow-[inset_2px_0_0_var(--primary)]",
              )}
            >
              <TableCell>
                <Mono dim>{run.runId}</Mono>
              </TableCell>
              <TableCell className="font-medium">{run.agentId}</TableCell>
              <TableCell>
                {/* Dot and word together: the state has to survive a
                    monochrome print and a colour-blind reader. */}
                <span className="flex items-center gap-2">
                  <StateDot state={state} />
                  {t(PHASE_LABELS[run.phase])}
                </span>
              </TableCell>
              <TableCell className={NUM}>{run.seq}</TableCell>
              <TableCell className={NUM}>
                {formatDuration(run.startedAt, run.endedAt)}
              </TableCell>
              <TableCell className={NUM}>{formatCost(run.cost)}</TableCell>
              <TableCell className={`${NUM} text-muted-foreground`}>
                {formatRelative(run.startedAt)}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
