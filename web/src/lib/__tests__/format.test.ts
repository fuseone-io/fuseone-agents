import { describe, expect, it } from "vitest";
import { formatMicros, formatRelative } from "@/lib/format";

describe("formatMicros", () => {
  it("keeps sub-cent amounts visible instead of rounding them to zero", () => {
    // A single run often costs a fraction of a cent. Rounding it to R$ 0,00
    // tells the reader the platform is free, which is the one thing a FinOps
    // view must never imply.
    expect(formatMicros(2_400)).not.toMatch(/^R\$\s?0,00$/);
  });

  it("renders ordinary amounts with two decimals", () => {
    expect(formatMicros(1_500_000)).toMatch(/1,50/);
  });

  it("treats zero as zero", () => {
    expect(formatMicros(0)).toMatch(/0,00/);
  });
});

describe("formatRelative", () => {
  it("describes a past instant as elapsed, not upcoming", () => {
    const now = Date.parse("2026-08-11T12:00:00Z");
    expect(formatRelative("2026-08-11T09:00:00Z", now)).toContain("há");
  });
});
