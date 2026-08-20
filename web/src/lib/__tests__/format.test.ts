import { beforeEach, describe, expect, it } from "vitest";
import { setLocale } from "@/i18n";
import {
  DEFAULT_CURRENCY,
  formatMicros,
  formatRelative,
  setInstallationCurrency,
} from "@/lib/format";

describe("formatMicros", () => {
  beforeEach(() => {
    setLocale("pt-BR");
    setInstallationCurrency(DEFAULT_CURRENCY);
  });

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

  it("changes the separators with the language and never the currency", () => {
    // The locale says how a number reads; the installation says what it bills
    // in. A reader who switched to English and saw dollars would be looking at
    // a different number.
    setLocale("en-US");
    const english = formatMicros(1_234_500_000);
    setLocale("pt-BR");
    const portuguese = formatMicros(1_234_500_000);

    expect(english).toContain("1,234.50");
    expect(portuguese).toContain("1.234,50");
    expect(english).toContain("R$");
    expect(portuguese).toContain("R$");
  });

  it("uses the installation currency rather than assuming Brazilian reais", () => {
    setInstallationCurrency("USD");

    expect(formatMicros(1_500_000)).toMatch(/US\$/);
  });
});

describe("formatRelative", () => {
  const past = "2026-08-11T09:00:00Z";
  const now = Date.parse("2026-08-11T12:00:00Z");

  it("describes a past instant as elapsed, not upcoming", () => {
    setLocale("pt-BR");
    expect(formatRelative(past, now)).toContain("há");
  });

  it("says the same thing in English", () => {
    setLocale("en-US");
    expect(formatRelative(past, now)).toContain("ago");
  });
});
