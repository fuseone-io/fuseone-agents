import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  FilterSelect,
  type FilterOption,
} from "@/components/shared/filter-select";
import { useAgents } from "@/features/agents/api";
import type { MeasureFilters } from "@/features/work-items/api";

const WEEKS: FilterOption[] = [
  { value: "4", label: "workItems.measures.weeks4" },
  { value: "8", label: "workItems.measures.weeks8" },
  { value: "12", label: "workItems.measures.weeks12" },
  { value: "26", label: "workItems.measures.weeks26" },
];
const ALL = "all";

export interface MeasureView extends MeasureFilters {
  weeks: number;
}

/**
 * How many weeks, and optionally one area and one role. The choices come from
 * the agents the reader can see in the active scope, not from the rows: rows
 * narrowed to one role would leave only that role to pick.
 */
export function WorkItemMeasuresFilters({
  view,
  onChange,
}: {
  view: MeasureView;
  onChange: (view: MeasureView) => void;
}) {
  const { t } = useTranslation();
  const { data } = useAgents();
  const { areas, agents } = useMemo(() => {
    const items = data?.items ?? [];
    const inArea = items.filter((a) => !view.area || a.scope.area === view.area);
    return {
      areas: [...new Set(items.map((a) => a.scope.area))].sort(),
      agents: [...new Set(inArea.map((a) => a.agentId))].sort(),
    };
  }, [data, view.area]);
  const options = (all: string, values: string[]) => [
    { value: ALL, label: t(all) },
    ...values.map((v) => ({ value: v, label: v })),
  ];
  const pick = (value: string) => (value === ALL ? undefined : value);

  return (
    <>
      <FilterSelect
        label="workItems.measures.filterWeeks"
        value={String(view.weeks)}
        options={WEEKS}
        onChange={(v) => onChange({ ...view, weeks: Number(v) })}
        width={130}
      />
      <FilterSelect
        label="workItems.measures.filterArea"
        value={view.area ?? ALL}
        options={options("workItems.measures.allAreas", areas)}
        onChange={(v) => onChange({ ...view, area: pick(v), agentId: undefined })}
        width={160}
      />
      <FilterSelect
        label="workItems.measures.filterAgent"
        value={view.agentId ?? ALL}
        options={options("workItems.measures.allAgents", agents)}
        onChange={(v) => onChange({ ...view, agentId: pick(v) })}
        width={170}
      />
    </>
  );
}
