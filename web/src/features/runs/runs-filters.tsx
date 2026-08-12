import { useTranslation } from "react-i18next";
import { Toolbar } from "@/components/shared/toolbar";
import {
  FilterSelect,
  type FilterOption,
} from "@/components/shared/filter-select";
import type { Phase } from "@/lib/api/client";

const PHASES: FilterOption[] = [
  { value: "all", label: "runs.allStates" },
  { value: "running", label: "runs.phaseRunning" },
  { value: "awaiting_approval", label: "runs.phaseAwaitingApproval" },
  { value: "awaiting_tool", label: "runs.phaseAwaitingTool" },
  { value: "parked", label: "runs.phaseParked" },
  { value: "finished", label: "runs.phaseFinished" },
];

const PERIODS: FilterOption[] = [
  { value: "1", label: "runs.last24h" },
  { value: "7", label: "runs.last7d" },
  { value: "30", label: "runs.last30d" },
  { value: "all", label: "runs.wholePeriod" },
];

const DAYS: Record<string, number | null> = {
  "1": 1,
  "7": 7,
  "30": 30,
  all: null,
};
const DAY_MS = 24 * 60 * 60 * 1000;
const MINUTE_MS = 60 * 1000;

/** Every filter here is applied by the server, so a page of results is the
 *  whole answer rather than whatever happened to be loaded. */
export function RunsFilters({
  search,
  phase,
  period,
  onSearch,
  onPhase,
  onPeriod,
}: {
  search: string;
  phase: Phase | "all";
  period: string;
  onSearch: (value: string) => void;
  onPhase: (value: Phase | "all") => void;
  onPeriod: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Toolbar
      placeholder={t("runs.searchPlaceholder")}
      value={search}
      onChange={onSearch}
    >
      <FilterSelect
        label={t("runs.filterByState")}
        value={phase}
        options={PHASES}
        onChange={(v) => onPhase(v as Phase | "all")}
        width={220}
      />
      <FilterSelect
        label={t("runs.filterByPeriod")}
        value={period}
        options={PERIODS}
        onChange={onPeriod}
        width={180}
      />
    </Toolbar>
  );
}

/**
 * sinceFor turns the chosen period into the instant the API filters on.
 *
 * Rounded down to the minute, and that rounding is load-bearing rather than
 * cosmetic: the result becomes a query key, so a value that moved with the
 * clock would produce a new key on every render, refetch, re-render, and hammer
 * the API in a loop that never settles.
 */
export function sinceFor(period: string, now = Date.now()): string | undefined {
  const days = DAYS[period];
  if (days == null) return undefined;

  const bucket = Math.floor(now / MINUTE_MS) * MINUTE_MS;
  return new Date(bucket - days * DAY_MS).toISOString();
}
