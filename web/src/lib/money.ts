const MICROS_PER_UNIT = 1_000_000;

/**
 * An amount a person typed, in the installation's currency, as micros.
 *
 * Money crosses the wire as integer millionths, and nobody should have to type
 * them: "500000000" and "50000000" differ by a zero a reader does not count.
 * A comma or a point is the decimal separator, whichever the person's
 * keyboard gives. Empty is zero, which every ceiling reads as "no ceiling".
 * Anything else that is not a plain non-negative number is null rather than a
 * guess, so a caller keeps the last good value instead of saving a wrong one.
 */
export function microsFromText(text: string): number | null {
  const trimmed = text.trim();
  if (trimmed === "") return 0;
  if (!/^\d+([.,]\d*)?$/.test(trimmed)) return null;
  return Math.round(Number(trimmed.replace(",", ".")) * MICROS_PER_UNIT);
}

/** Stored micros as the amount a person would type; zero is left empty. */
export function textFromMicros(micros: number | undefined): string {
  return micros ? String(micros / MICROS_PER_UNIT) : "";
}
