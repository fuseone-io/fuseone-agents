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
  // A micro is the finest unit there is, so a seventh decimal is refused
  // rather than rounded away. The digits are combined as integers: a float
  // product loses the last micro long before the amount is large.
  const parts = /^(\d+)(?:[.,](\d{0,6}))?$/.exec(trimmed);
  if (!parts) return null;
  const [, whole = "0", fraction = ""] = parts;
  const micros =
    BigInt(whole) * BigInt(MICROS_PER_UNIT) + BigInt(fraction.padEnd(6, "0"));
  if (micros > BigInt(Number.MAX_SAFE_INTEGER)) return null;
  return Number(micros);
}

/**
 * Stored micros as the amount a person would type; zero is left empty.
 *
 * Built from the integer quotient and remainder, not a float division, which
 * showed the largest exact amount one micro high — text this module's own
 * reader then refused.
 */
export function textFromMicros(micros: number | undefined): string {
  if (!micros) return "";
  const remainder = micros % MICROS_PER_UNIT;
  const whole = (micros - remainder) / MICROS_PER_UNIT;
  const fraction = String(remainder).padStart(6, "0").replace(/0+$/, "");
  return fraction ? `${whole}.${fraction}` : String(whole);
}
