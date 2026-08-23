import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { LoadMore } from "@/components/shared/load-more";
import { Mono } from "@/components/shared/mono";
import { formatMicros, formatTokens } from "@/lib/format";
import { useVisibleItems } from "@/hooks/use-visible-items";
import type { components } from "@/lib/api/schema.gen";

type PlanningSpendRollup = components["schemas"]["PlanningSpendRollup"];
type PlanningSpendBucket = components["schemas"]["PlanningSpendBucket"];
type Cost = components["schemas"]["Cost"];

const HEAD =
  "h-[30px] bg-muted text-2xs uppercase tracking-label text-muted-foreground";
const NUM = "text-right font-mono text-xs tabular-nums";

export function PlanningSpendTable({
  title,
  empty,
  rollup,
  cut,
}: {
  title: string;
  empty: string;
  rollup?: PlanningSpendRollup;
  cut: "agent" | "model";
}) {
  const { t } = useTranslation();
  const buckets = rollup?.buckets ?? [];
  const page = useVisibleItems(buckets, 8);

  return (
    <div className="min-w-0 rounded-md border">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
        <div className="text-sm font-medium">{title}</div>
        {rollup && (
          <div className="flex flex-wrap items-center justify-end gap-2 text-xs text-muted-foreground">
            <span>
              {t("cost.planningTotal", {
                calls: rollup.calls,
                cost: formatMicros(rollup.total.micros),
                tokens: formatTokens(costTokens(rollup.total)),
              })}
            </span>
            {rollup.unpriced > 0 && (
              <Badge variant="outline" className="text-warning">
                {t("cost.unpricedCalls", { count: rollup.unpriced })}
              </Badge>
            )}
          </div>
        )}
      </div>
      {page.visible.length === 0 ? (
        <p className="px-4 py-6 text-sm text-muted-foreground">{empty}</p>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className={HEAD}>{title}</TableHead>
                <TableHead className={`${HEAD} text-right`}>
                  {t("cost.calls")}
                </TableHead>
                <TableHead className={`${HEAD} text-right`}>
                  {t("cost.tokens")}
                </TableHead>
                <TableHead className={`${HEAD} text-right`}>
                  {t("cost.cost")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.visible.map((bucket) => (
                <TableRow
                  key={rowKey(bucket)}
                  className="h-12 border-border-subtle"
                >
                  <TableCell className="min-w-0">
                    {bucketName(bucket, cut)}
                  </TableCell>
                  <TableCell className={NUM}>{bucket.calls}</TableCell>
                  <TableCell className={NUM}>
                    <span className="inline-flex items-center justify-end gap-2">
                      <span>{formatTokens(costTokens(bucket.cost))}</span>
                      {bucket.unpriced > 0 && (
                        <Badge variant="outline" className="text-warning">
                          {t("cost.unpricedCalls", {
                            count: bucket.unpriced,
                          })}
                        </Badge>
                      )}
                    </span>
                  </TableCell>
                  <TableCell className={NUM}>
                    {formatMicros(bucket.cost.micros)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <div className="px-4 pb-3">
            <LoadMore
              loaded={page.loaded}
              total={page.total}
              hasMore={page.hasMore}
              isLoading={false}
              onLoad={page.loadMore}
            />
          </div>
        </>
      )}
    </div>
  );
}

function bucketName(bucket: PlanningSpendBucket, cut: "agent" | "model"): ReactNode {
  if (cut === "agent") return <Mono>{bucket.agent}</Mono>;
  return (
    <span className="flex min-w-0 flex-col gap-0.5">
      <Mono>{bucket.model}</Mono>
      <span className="text-xs text-muted-foreground">{bucket.provider}</span>
    </span>
  );
}

function costTokens(cost: Cost): number {
  return (
    (cost.inputTokens ?? 0) +
    (cost.outputTokens ?? 0) +
    (cost.cacheReadTokens ?? 0) +
    (cost.cacheWriteTokens ?? 0)
  );
}

function rowKey(bucket: PlanningSpendBucket): string {
  return bucket.agent || `${bucket.provider}/${bucket.model}`;
}
