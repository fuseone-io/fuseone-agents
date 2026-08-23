import { useTranslation } from "react-i18next";
import { Panel } from "@/components/shared/panel";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { formatInstant } from "@/lib/format";
import { PlanningSpendTable } from "@/features/cost/planning-spend-table";
import type { components } from "@/lib/api/schema.gen";

type PlanningSpendRollup = components["schemas"]["PlanningSpendRollup"];

export function PlanningSpendPanel({
  byModel,
  byAgent,
  isLoading,
  error,
  onRetry,
}: {
  byModel?: PlanningSpendRollup;
  byAgent?: PlanningSpendRollup;
  isLoading?: boolean;
  error?: unknown;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  if (!byModel && !byAgent && !isLoading && !error) return null;

  const partialFrom = partialProjectionStart(byModel) ?? partialProjectionStart(byAgent);

  return (
    <Panel
      title={t("cost.planningSpend")}
      action={
        partialFrom ? (
          <span className="text-xs text-muted-foreground">
            {t("cost.projectedSince", {
              date: formatInstant(partialFrom),
            })}
          </span>
        ) : null
      }
    >
      {error ? (
        <ErrorState error={error} onRetry={onRetry} />
      ) : isLoading && !byModel && !byAgent ? (
        <LoadingRows rows={4} />
      ) : (
        <>
          <p className="mb-4 max-w-[80ch] text-xs text-muted-foreground">
            {t("cost.planningSpendHint")}
          </p>
          <div className="grid min-w-0 gap-4 xl:grid-cols-2">
            <PlanningSpendTable
              cut="model"
              title={t("cost.byModel")}
              empty={t("cost.noPlanningSpend")}
              rollup={byModel}
            />
            <PlanningSpendTable
              cut="agent"
              title={t("cost.byAgent")}
              empty={t("cost.noPlanningSpend")}
              rollup={byAgent}
            />
          </div>
        </>
      )}
    </Panel>
  );
}

function partialProjectionStart(rollup?: PlanningSpendRollup): string | undefined {
  if (!rollup?.projectedFrom) return undefined;
  const from = Date.parse(rollup.from);
  const projected = Date.parse(rollup.projectedFrom);
  if (!Number.isFinite(from) || !Number.isFinite(projected)) return undefined;
  return projected > from ? rollup.projectedFrom : undefined;
}
