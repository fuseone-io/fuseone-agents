import { useTranslation } from "react-i18next";
import {
  FilterSelect,
  type FilterOption,
} from "@/components/shared/filter-select";
import type { MeasureFilters } from "@/features/work-items/api";
import type { MeasureChoices } from "@/features/work-items/use-measure-choices";

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
 * How many weeks, and optionally one area and one role of the active scope.
 * Shows the filters as they will be sent, so a choice the scope does not have
 * reads as "all" rather than as a value nobody is asking for.
 */
export function WorkItemMeasuresFilters({
  weeks,
  choices,
  onChange,
}: {
  weeks: number;
  choices: MeasureChoices;
  onChange: (view: MeasureView) => void;
}) {
  const { t } = useTranslation();
  const view = { weeks, ...choices.filters };
  const options = (all: string, values: string[]) => [
    { value: ALL, label: t(all) },
    ...values.map((v) => ({ value: v, label: v })),
  ];
  const pick = (value: string) => (value === ALL ? undefined : value);

  return (
    <>
      <FilterSelect
        label="workItems.measures.filterWeeks"
        value={String(weeks)}
        options={WEEKS}
        onChange={(v) => onChange({ ...view, weeks: Number(v) })}
        width={130}
      />
      <FilterSelect
        label="workItems.measures.filterArea"
        value={view.area ?? ALL}
        options={options("workItems.measures.allAreas", choices.areas)}
        onChange={(v) => onChange({ ...view, area: pick(v), agentId: undefined })}
        width={160}
      />
      <FilterSelect
        label="workItems.measures.filterAgent"
        value={view.agentId ?? ALL}
        options={options("workItems.measures.allAgents", choices.agents)}
        onChange={(v) => onChange({ ...view, agentId: pick(v) })}
        width={170}
      />
    </>
  );
}
