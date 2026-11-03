import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { currentLocale } from "@/i18n";
import type { MeasureWeek } from "@/features/work-items/measure-ratios";
import { WorkItemMeasuresRow } from "@/features/work-items/work-item-measures-row";

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";

const COLUMNS = [
  ["workItems.columnRole", false],
  ["workItems.columnRuns", true],
  ["workItems.measures.columnCards", true],
  ["workItems.columnRequested", true],
  ["workItems.columnApproved", true],
  ["workItems.columnRefused", true],
  ["workItems.measures.columnPending", true],
  ["workItems.columnAbandoned", true],
  ["workItems.measures.columnRevised", true],
  ["workItems.measures.columnRepeated", true],
  ["workItems.measures.columnStanding", true],
  ["workItems.measures.columnCostOnCards", true],
  ["workItems.measures.columnCostInherited", true],
  ["workItems.measures.columnCostOffCard", true],
  ["workItems.measures.columnCostPerCard", true],
] as const;

/**
 * The week's Monday, in UTC: weeks are cut there, and a reader west of
 * Greenwich would otherwise see every week start on a Sunday.
 */
const weekLabel = (week: string) =>
  new Intl.DateTimeFormat(currentLocale(), {
    dateStyle: "short",
    timeZone: "UTC",
  }).format(new Date(week));

/** One ISO week, a row per role that ran in it. */
export function WorkItemMeasuresWeek({
  week,
  forbidsStanding,
}: {
  week: MeasureWeek;
  /** Agents whose own policy forbids standing approvals. */
  forbidsStanding: ReadonlySet<string>;
}) {
  const { t } = useTranslation();
  const heading = useId();
  return (
    <section aria-labelledby={heading} className="flex flex-col">
      <div className="flex items-center gap-2 px-4 py-2">
        <h3 id={heading} className="text-sm font-medium">
          {t("workItems.measures.weekOf", { date: weekLabel(week.week) })}
        </h3>
        {!week.complete && (
          <Badge variant="outline">{t("workItems.measures.partial")}</Badge>
        )}
      </div>
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
          {week.rows.map((row) => (
            <WorkItemMeasuresRow
              key={row.agentId}
              row={row}
              forbidsStanding={forbidsStanding.has(row.agentId)}
            />
          ))}
        </TableBody>
      </Table>
    </section>
  );
}
