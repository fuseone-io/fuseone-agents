import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Panel } from "@/components/shared/panel";
import { Separator } from "@/components/ui/separator";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { useWorkItemMeasures } from "@/features/work-items/api";
import { weeksNewestFirst } from "@/features/work-items/measure-ratios";
import {
  WorkItemMeasuresFilters,
  type MeasureView,
} from "@/features/work-items/work-item-measures-filters";
import { WorkItemMeasuresWeek } from "@/features/work-items/work-item-measures-week";
import { WorkItemUnavailable } from "@/features/work-items/work-item-unavailable";

const DEFAULT_VIEW: MeasureView = { weeks: 8 };

/**
 * Per role, per week: what the ledger knows about the factory's work (#201).
 * A table per ISO week, the current one first and marked partial, and under
 * them, in words, what these counts cannot say.
 */
export function WorkItemMeasuresPanel() {
  const { t } = useTranslation();
  const [view, setView] = useState<MeasureView>(DEFAULT_VIEW);
  const { weeks, ...filters } = view;
  const measures = useWorkItemMeasures(weeks, filters);
  const shown = useMemo(
    () => weeksNewestFirst(measures.data?.rows ?? []),
    [measures.data],
  );

  return (
    <Panel
      title={t("workItems.measures.title")}
      action={<WorkItemMeasuresFilters view={view} onChange={setView} />}
      flush
    >
      {measures.isLoading ? (
        <div className="p-4">
          <LoadingRows rows={3} />
        </div>
      ) : measures.error ? (
        <div className="p-4">
          <ErrorState
            error={measures.error}
            onRetry={() => void measures.refetch()}
          />
        </div>
      ) : (
        <>
          {shown.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title={t("workItems.measures.empty")}
                hint={t("workItems.measures.emptyHint")}
              />
            </div>
          ) : (
            shown.map((week) => (
              <WorkItemMeasuresWeek key={week.week} week={week} />
            ))
          )}
          <Separator />
          <WorkItemUnavailable entries={measures.data?.unavailable ?? []} />
        </>
      )}
    </Panel>
  );
}
