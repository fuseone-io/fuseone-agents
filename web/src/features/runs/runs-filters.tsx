import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { Phase } from "@/lib/api/client";

const PHASES: { value: Phase | "all"; label: string }[] = [
  { value: "all", label: "Todas as situações" },
  { value: "running", label: "Em execução" },
  { value: "awaiting_approval", label: "Aguardando aprovação" },
  { value: "awaiting_tool", label: "Chamando ferramenta" },
  { value: "parked", label: "Estacionada" },
  { value: "finished", label: "Concluída" },
];

const PERIODS: { value: string; label: string; days: number | null }[] = [
  { value: "1", label: "Últimas 24 horas", days: 1 },
  { value: "7", label: "Últimos 7 dias", days: 7 },
  { value: "30", label: "Últimos 30 dias", days: 30 },
  { value: "all", label: "Todo o período", days: null },
];

const DAY_MS = 24 * 60 * 60 * 1000;
const MINUTE_MS = 60 * 1000;

/** Both filters are applied by the server, so a page of results is the whole
 *  answer rather than whatever happened to be loaded. */
export function RunsFilters({
  phase,
  period,
  onPhase,
  onPeriod,
}: {
  phase: Phase | "all";
  period: string;
  onPhase: (value: Phase | "all") => void;
  onPeriod: (value: string) => void;
}) {
  return (
    <div className="flex shrink-0 items-center gap-2">
      <Select value={phase} onValueChange={(v) => onPhase(v as Phase | "all")}>
        <SelectTrigger className="w-[220px]" aria-label="Filtrar por situação">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PHASES.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={period} onValueChange={onPeriod}>
        <SelectTrigger className="w-[180px]" aria-label="Filtrar por período">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PERIODS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
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
  const days = PERIODS.find((p) => p.value === period)?.days;
  if (days == null) return undefined;

  const bucket = Math.floor(now / MINUTE_MS) * MINUTE_MS;
  return new Date(bucket - days * DAY_MS).toISOString();
}
