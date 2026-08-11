import { Toolbar } from "@/components/shared/toolbar";
import { FilterSelect, type FilterOption } from "@/components/shared/filter-select";
import type { Phase } from "@/lib/api/client";

const PHASES: FilterOption[] = [
  { value: "all", label: "Todas as situações" },
  { value: "running", label: "Em execução" },
  { value: "awaiting_approval", label: "Aguardando aprovação" },
  { value: "awaiting_tool", label: "Chamando ferramenta" },
  { value: "parked", label: "Estacionada" },
  { value: "finished", label: "Concluída" },
];

const PERIODS: FilterOption[] = [
  { value: "1", label: "Últimas 24 horas" },
  { value: "7", label: "Últimos 7 dias" },
  { value: "30", label: "Últimos 30 dias" },
  { value: "all", label: "Todo o período" },
];

const DAYS: Record<string, number | null> = { "1": 1, "7": 7, "30": 30, all: null };
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
  return (
    <Toolbar placeholder="Buscar por execução ou agente" value={search} onChange={onSearch}>
      <FilterSelect
        label="Filtrar por situação"
        value={phase}
        options={PHASES}
        onChange={(v) => onPhase(v as Phase | "all")}
        width={220}
      />
      <FilterSelect
        label="Filtrar por período"
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
