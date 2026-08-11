/**
 * The two windows every comparison on this screen is made of.
 *
 * Rounded down to the hour, and derived once. A bound read straight from the
 * clock changes on every render, and these are TanStack Query keys: a moving
 * key means refetch, re-render, refetch, for as long as the page is open.
 */
export interface Window {
  since: string;
  until: string;
}

export interface Windows {
  /** Since the start of today, still open. */
  current: Window;
  /** The same span, one day earlier, closed. */
  previous: Window;
}

export function windowsFor(now = Date.now()): Windows {
  const start = new Date(now);
  start.setMinutes(0, 0, 0);
  start.setHours(0);

  const dayMS = 24 * 60 * 60 * 1000;
  const startedAt = start.getTime();

  return {
    current: { since: iso(startedAt), until: iso(startedAt + dayMS) },
    previous: { since: iso(startedAt - dayMS), until: iso(startedAt) },
  };
}

function iso(ms: number): string {
  return new Date(ms).toISOString();
}

/**
 * The change between two figures, as a share of the earlier one.
 *
 * Undefined rather than zero or infinity when there is nothing to compare
 * against: "+100%" from a base of zero is a number with no meaning, and a
 * reader would take it for a measurement.
 */
export function deltaOf(current: number, previous: number): number | undefined {
  if (previous === 0) return undefined;
  return (current - previous) / previous;
}
