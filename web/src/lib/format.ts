import { currentLocale } from "@/i18n";
import type { Cost } from "@/lib/api/client";

const MICROS_PER_UNIT = 1_000_000;
export const DEFAULT_CURRENCY = "BRL";
let installationCurrency = DEFAULT_CURRENCY;

/**
 * What the installation bills in.
 *
 * Deliberately not a function of the locale. Money crosses the wire as
 * millionths of the installation's currency, so choosing English must change
 * "1.234,50" into "1,234.50" and leave R$ as R$ — a reader who switched
 * language and saw dollars would be looking at a different number. The app
 * reads the installation currency from /money and sets it here.
 */
export function setInstallationCurrency(currency: string | undefined) {
  installationCurrency = normalizeCurrency(currency);
}

export function currentCurrency(): string {
  return installationCurrency;
}

/**
 * Built per call rather than once at module load.
 *
 * A formatter captured at import time is pinned to whichever locale was active
 * then, so every figure on the screen would keep the old language after a
 * switch. Intl caches internally, so this is not the cost it looks like.
 */
const numberFormat = (options: Intl.NumberFormatOptions) =>
  new Intl.NumberFormat(currentLocale(), options);

/**
 * A run costing a fraction of a cent is normal, so small amounts keep four
 * decimals instead of rounding to zero and reading as free.
 */
export function formatMicros(micros: number): string {
  return formatCurrencyMicros(micros, installationCurrency);
}

export function formatCurrencyMicros(micros: number, currency: string): string {
  const value = micros / MICROS_PER_UNIT;
  const digits =
    value !== 0 && Math.abs(value) < 0.01 ? { maximumFractionDigits: 4 } : {};
  return numberFormat({
    style: "currency",
    currency: normalizeCurrency(currency),
    ...digits,
  }).format(value);
}

export function formatCost(cost: Cost | undefined): string {
  return formatMicros(cost?.micros ?? 0);
}

function normalizeCurrency(currency: string | undefined): string {
  const code = currency?.trim().toUpperCase();
  return code && /^[A-Z]{3}$/.test(code) ? code : DEFAULT_CURRENCY;
}

export function formatTokens(n: number | undefined): string {
  return numberFormat({}).format(n ?? 0);
}

export function formatBytes(n: number | undefined): string {
  const bytes = n ?? 0;
  if (bytes < 1024) return `${numberFormat({}).format(bytes)} B`;
  if (bytes < 1024 * 1024) {
    return `${numberFormat({ maximumFractionDigits: 1 }).format(bytes / 1024)} KiB`;
  }
  return `${numberFormat({ maximumFractionDigits: 1 }).format(bytes / (1024 * 1024))} MiB`;
}

export function formatInstant(iso: string): string {
  return new Intl.DateTimeFormat(currentLocale(), {
    dateStyle: "short",
    timeStyle: "medium",
  }).format(new Date(iso));
}

/**
 * The time alone, for a sequence that happens within one day.
 *
 * A trail repeats its stamp on every row; printing the date fifteen times
 * makes the column read as noise and hides the thing it is there for, which is
 * how far apart two steps were.
 */
export function formatTime(iso: string): string {
  return new Intl.DateTimeFormat(currentLocale(), {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(iso));
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["second", 60],
  ["minute", 60],
  ["hour", 24],
  ["day", 7],
];

export function formatRelative(iso: string, now = Date.now()): string {
  const relative = new Intl.RelativeTimeFormat(currentLocale(), {
    numeric: "auto",
  });
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
 * column harder to scan. The unit letters are the same in both languages the
 * console ships, which is why this is not a translated string.
 */
export function formatDuration(
  startedAt: string,
  endedAt?: string | null,
  now = Date.now(),
): string {
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
