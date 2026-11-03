import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { TableCell, TableRow } from "@/components/ui/table";
import { Mono } from "@/components/shared/mono";
import {
  costPerCard,
  formatRatio,
  rowRatios,
} from "@/features/work-items/measure-ratios";
import { WorkItemCost } from "@/features/work-items/work-item-cost";
import type { WorkItemMeasureRow } from "@/lib/api/client";

const NUM = "text-right font-mono text-xs tabular-nums";

/**
 * One role's week. Every rate sits beside the count it is a share of, so a
 * reader never has to trust a percentage of a number they cannot see.
 */
export function WorkItemMeasuresRow({ row }: { row: WorkItemMeasureRow }) {
  const { t } = useTranslation();
  const p = row.proposals;
  const rates = rowRatios(row);
  return (
    <TableRow className="h-10 border-border-subtle">
      <TableCell className="font-medium">{row.agentId}</TableCell>
      <TableCell className={NUM}>
        {row.runs}
        {row.openRuns > 0 && (
          <Mono dim className="ml-1.5">
            {t("workItems.measures.openRuns", { count: row.openRuns })}
          </Mono>
        )}
      </TableCell>
      <TableCell className={NUM}>{row.cards}</TableCell>
      <TableCell className={NUM}>{p.requested}</TableCell>
      <Counted count={p.approved} share={rates.approved} />
      <Counted count={p.refused} share={rates.refused} />
      <TableCell className={NUM}>{p.pending}</TableCell>
      <TableCell className={NUM}>{p.abandoned}</TableCell>
      <Counted count={p.revisedAfterRefusal} share={rates.revised} />
      <Counted count={p.repeatedAfterRefusal} share={rates.repeated} />
      <TableCell className={NUM}>
        {p.standing}
        {p.standing > 0 && (
          <Badge className="ml-1.5 h-5 bg-warning-surface text-warning">
            <TriangleAlert aria-hidden />
            {t("workItems.measures.standingForbidden")}
          </Badge>
        )}
      </TableCell>
      <TableCell className={NUM}>
        <WorkItemCost micros={row.costOnCardsMicros} />
      </TableCell>
      <TableCell className={NUM}>
        <WorkItemCost micros={row.costInheritedMicros} />
      </TableCell>
      <TableCell className={NUM}>
        <WorkItemCost micros={row.costOffCardMicros} />
      </TableCell>
      <TableCell className={NUM}>
        <PerCard row={row} />
      </TableCell>
    </TableRow>
  );
}

/** A count and, beside it, its share — or a dash when there was no whole. */
function Counted({ count, share }: { count: number; share?: number }) {
  const { t } = useTranslation();
  return (
    <TableCell className={NUM}>
      {count}
      <Mono dim className="ml-1.5">
        {share === undefined ? (
          <>
            <span aria-hidden>—</span>
            <span className="sr-only">{t("workItems.measures.noWhole")}</span>
          </>
        ) : (
          formatRatio(share)
        )}
      </Mono>
    </TableCell>
  );
}

/** Hidden like every cost when the reader may not see it; a dash with no card. */
function PerCard({ row }: { row: WorkItemMeasureRow }) {
  const { t } = useTranslation();
  if (row.costOnCardsMicros === undefined) return <WorkItemCost />;
  const micros = costPerCard(row);
  if (micros !== undefined) return <WorkItemCost micros={micros} />;
  return (
    <Mono dim>
      <span aria-hidden>—</span>
      <span className="sr-only">{t("workItems.measures.noCards")}</span>
    </Mono>
  );
}
