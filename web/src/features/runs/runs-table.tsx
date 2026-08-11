import { useNavigate } from "react-router-dom";
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
import type { Run } from "@/lib/api/client";

const HEAD = "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";
const NUM = "text-right font-mono text-xs tabular-nums";

/**
 * The executions table.
 *
 * Everything the platform produced reads in mono with fixed-width digits and
 * sits on the right, so the four numeric columns can be compared down the page
 * without reading each cell.
 */
export function RunsTable({ runs }: { runs: Run[] }) {
  const navigate = useNavigate();

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className={HEAD}>Execução</TableHead>
          <TableHead className={HEAD}>Agente</TableHead>
          <TableHead className={HEAD}>Situação</TableHead>
          <TableHead className={`${HEAD} text-right`}>Passos</TableHead>
          <TableHead className={`${HEAD} text-right`}>Duração</TableHead>
          <TableHead className={`${HEAD} text-right`}>Custo</TableHead>
          <TableHead className={`${HEAD} text-right`}>Início</TableHead>
        </TableRow>
      </TableHeader>

      <TableBody>
        {runs.map((run) => {
          const state = stateOfPhase(run.phase);
          return (
            <TableRow
              key={run.runId}
              onClick={() => navigate(`/runs/${run.runId}`)}
              className="h-10 cursor-pointer border-border-subtle"
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
                  {PHASE_LABELS[run.phase]}
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
