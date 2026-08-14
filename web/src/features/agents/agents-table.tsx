import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
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
import { StageBadge } from "@/features/agents/stage-badge";
import { stateOfAgent, STATE_TEXT } from "@/lib/agent-state";
import { formatMicros, formatRelative } from "@/lib/format";
import type { Agent } from "@/lib/api/client";

/**
 * The fleet as rows.
 *
 * The same facts a card carries, in the order somebody scanning a column reads
 * them: which agent, where it lives, how far it is trusted, and then the three
 * numbers. A card answers "how is this one doing"; this answers "which one",
 * which is the question that arrives once there are more than a screenful.
 */
export function AgentsTable({ agents }: { agents: Agent[] }) {
  const { t } = useTranslation();
  const head = "h-8 text-2xs font-medium uppercase tracking-wide";

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className={head}>{t("agents.agent")}</TableHead>
          <TableHead className={head}>{t("cost.area")}</TableHead>
          <TableHead className={head}>{t("agents.stage")}</TableHead>
          <TableHead className={`${head} text-right`}>
            {t("agents.runs")}
          </TableHead>
          <TableHead className={`${head} text-right`}>
            {t("agents.finished")}
          </TableHead>
          <TableHead className={`${head} text-right`}>
            {t("agents.ceiling")}
          </TableHead>
          <TableHead className={head}>{t("agents.lastRunColumn")}</TableHead>
        </TableRow>
      </TableHeader>

      <TableBody>
        {agents.map((agent) => (
          <Row key={`${agent.agentId}@${agent.versionId}`} agent={agent} />
        ))}
      </TableBody>
    </Table>
  );
}

function Row({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const state = stateOfAgent(agent.activity?.lastPhase);
  const runs = agent.activity?.runs ?? 0;
  const finished = agent.activity?.finished ?? 0;

  return (
    <TableRow className="h-10 border-border-subtle">
      <TableCell>
        <Link
          to={`/agents/${agent.agentId}`}
          className="flex items-center gap-2 hover:underline"
        >
          {/* Colour never carries the meaning: the dot repeats what the last
              run's phase already says in the last column. */}
          <StateDot state={state} />
          <span className="font-medium">{agent.name}</span>
          <Mono dim className="text-2xs">
            {agent.agentId}
          </Mono>
        </Link>
      </TableCell>
      <TableCell className="text-muted-foreground">
        {agent.scope.area}
      </TableCell>
      <TableCell>{agent.stage && <StageBadge stage={agent.stage} />}</TableCell>
      <TableCell className="text-right tabular-nums">{runs}</TableCell>
      <TableCell className="text-right tabular-nums">
        {runs > 0 ? `${Math.round((finished / runs) * 100)}%` : "—"}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {agent.budget.micros ? formatMicros(agent.budget.micros) : "—"}
      </TableCell>
      <TableCell className={`text-xs ${STATE_TEXT[state]}`}>
        {agent.activity?.lastRunAt
          ? formatRelative(agent.activity.lastRunAt)
          : t("agents.neverRanShort")}
      </TableCell>
    </TableRow>
  );
}
