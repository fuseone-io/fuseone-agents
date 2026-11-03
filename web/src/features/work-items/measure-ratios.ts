import { currentLocale } from "@/i18n";
import type { WorkItemMeasureRow } from "@/lib/api/client";

/**
 * A share, or nothing when there is nothing to share out.
 *
 * Nothing rather than zero: "0 %" of no decisions reads as a role whose every
 * proposal was turned down, and NaN reads as a broken screen. The caller shows
 * a dash.
 */
export function ratio(part: number, whole: number): number | undefined {
  return whole > 0 ? part / whole : undefined;
}

export function formatRatio(value: number): string {
  return new Intl.NumberFormat(currentLocale(), {
    style: "percent",
    maximumFractionDigits: 0,
  }).format(value);
}

/** Decided is what a person answered: approved plus refused. */
export function rowRatios(row: WorkItemMeasureRow) {
  const { approved, refused, revisedAfterRefusal, repeatedAfterRefusal } =
    row.proposals;
  const decided = approved + refused;
  return {
    approved: ratio(approved, decided),
    refused: ratio(refused, decided),
    revised: ratio(revisedAfterRefusal, refused),
    repeated: ratio(repeatedAfterRefusal, refused),
  };
}

/**
 * Cost on cards over cards, in micros. Absent when the cost is — the reader
 * may not see it — and when no card was claimed, which divides by nothing.
 */
export function costPerCard(row: WorkItemMeasureRow): number | undefined {
  if (row.costOnCardsMicros === undefined || row.cards === 0) return undefined;
  return Math.round(row.costOnCardsMicros / row.cards);
}

export interface MeasureWeek {
  week: string;
  complete: boolean;
  rows: WorkItemMeasureRow[];
}

/**
 * The server's rows, oldest week first, folded into weeks newest first: the
 * week a reader is living in is the one they came to look at. Agents keep the
 * server's order inside a week.
 */
export function weeksNewestFirst(rows: WorkItemMeasureRow[]): MeasureWeek[] {
  const byWeek = new Map<string, MeasureWeek>();
  for (const row of rows) {
    const week = byWeek.get(row.week) ?? {
      week: row.week,
      complete: true,
      rows: [],
    };
    week.complete = week.complete && row.complete;
    week.rows.push(row);
    byWeek.set(row.week, week);
  }
  return [...byWeek.values()].sort((a, b) => b.week.localeCompare(a.week));
}
