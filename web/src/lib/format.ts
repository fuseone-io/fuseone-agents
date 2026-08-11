import type { Cost } from "@/lib/api/client";

const MICROS_PER_UNIT = 1_000_000;

const currency = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
});

const compactCurrency = new Intl.NumberFormat("pt-BR", {
  style: "currency",
  currency: "BRL",
  maximumFractionDigits: 4,
});

/**
 * Money crosses the wire as an integer in millionths of the installation's
 * currency and is only ever divided here, at the edge. A run costing a
 * fraction of a cent is normal, so small amounts keep four decimals instead of
 * rounding to R$ 0,00 and reading as free.
 */
export function formatMicros(micros: number): string {
  const value = micros / MICROS_PER_UNIT;
  if (value !== 0 && Math.abs(value) < 0.01) {
    return compactCurrency.format(value);
  }
  return currency.format(value);
}

export function formatCost(cost: Cost | undefined): string {
  return formatMicros(cost?.micros ?? 0);
}

const integer = new Intl.NumberFormat("pt-BR");

export function formatTokens(n: number | undefined): string {
  return integer.format(n ?? 0);
}

const dateTime = new Intl.DateTimeFormat("pt-BR", {
  dateStyle: "short",
  timeStyle: "medium",
});

export function formatInstant(iso: string): string {
  return dateTime.format(new Date(iso));
}

const relative = new Intl.RelativeTimeFormat("pt-BR", { numeric: "auto" });

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["second", 60],
  ["minute", 60],
  ["hour", 24],
  ["day", 7],
];

export function formatRelative(iso: string, now = Date.now()): string {
  let delta = (new Date(iso).getTime() - now) / 1000;
  for (const [unit, step] of UNITS) {
    if (Math.abs(delta) < step) {
      return relative.format(Math.round(delta), unit);
    }
    delta /= step;
  }
  return relative.format(Math.round(delta), "week");
}

/** Truncates a hash for display while keeping enough to compare by eye. */
export function shortHash(hash: string): string {
  return hash.slice(0, 12);
}

/**
 * How long a run took, or how long it has been going.
 *
 * Rendered coarsely on purpose — "2m 41s", not "2m 41.283s". The reader is
 * comparing runs in a column, and precision they cannot act on only makes the
 * column harder to scan.
 */
export function formatDuration(startedAt: string, endedAt?: string | null, now = Date.now()): string {
  const start = Date.parse(startedAt);
  const end = endedAt ? Date.parse(endedAt) : now;
  if (Number.isNaN(start) || Number.isNaN(end)) return "—";
  return formatDurationMs(end - start);
}

/** The same reading, for a duration the server already measured. */
export function formatDurationMs(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds}s`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;

  const hours = Math.floor(minutes / 60);
  return `${hours}h ${minutes % 60}m`;
}
