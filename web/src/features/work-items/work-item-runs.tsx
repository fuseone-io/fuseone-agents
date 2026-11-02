import { Link } from "react-router-dom";
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
import { formatInstant } from "@/lib/format";
import type { WorkItemRun } from "@/lib/api/client";

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";
const NUM = "text-right font-mono text-xs tabular-nums";

const COLUMNS = [
  ["workItems.columnClaimed", false],
  ["runs.columnRun", false],
  ["runs.columnAgent", false],
  ["workItems.columnArea", false],
  ["runs.columnState", false],
  ["workItems.columnLink", false],
  ["workItems.columnEnded", true],
] as const;

/**
 * How a run is on the trail: it claimed the card, or a pull request linked to
 * it — named, so the reader sees which.
 */
function RunLink({ run }: { run: WorkItemRun }) {
  const { t } = useTranslation();
  if (run.link !== "alias") return <>{t("workItems.linkClaim")}</>;
  return (
    <span className="flex items-center gap-2">
      {t("workItems.linkAlias")}
      <Mono>{run.ref}</Mono>
    </span>
  );
}

/**
 * The card's runs, oldest claim first. Each leads to the run's own screen,
 * where its trail is; nothing of the trail itself is repeated here.
 */
export function WorkItemRuns({ runs }: { runs: WorkItemRun[] }) {
  const { t } = useTranslation();
  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          {COLUMNS.map(([key, numeric]) => (
            <TableHead
              key={key}
              className={numeric ? `${HEAD} text-right` : HEAD}
            >
              {t(key)}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {runs.map((run) => (
          <TableRow key={run.runId} className="h-10 border-border-subtle">
            <TableCell className="font-mono text-xs tabular-nums">
              {formatInstant(run.claimedAt)}
            </TableCell>
            <TableCell>
              <Link
                to={`/runs/${run.runId}`}
                className="rounded-sm underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/26"
              >
                <Mono>{run.runId}</Mono>
              </Link>
            </TableCell>
            <TableCell className="font-medium">{run.agentId}</TableCell>
            <TableCell>{run.scope.area}</TableCell>
            <TableCell>
              <span className="flex items-center gap-2">
                <StateDot state={stateOfPhase(run.phase)} />
                {t(PHASE_LABELS[run.phase])}
              </span>
            </TableCell>
            <TableCell>
              <RunLink run={run} />
            </TableCell>
            <TableCell className={NUM}>
              {run.endedAt
                ? formatInstant(run.endedAt)
                : t("workItems.notEnded")}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
