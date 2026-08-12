import { describe, expect, it } from "vitest";
import ptBR from "@/i18n/pt-BR.json";
import enUS from "@/i18n/en-US.json";

/**
 * Parity is the rule web/CLAUDE.md states, and it is not pedantry: a key
 * present in one catalogue and missing from the other renders its own name to
 * whoever chose the other language. The failure is invisible to whoever added
 * the key, because they were reading their own locale.
 */
const keysOf = (catalogue: object, prefix = ""): string[] =>
  Object.entries(catalogue).flatMap(([key, value]) =>
    typeof value === "object" && value !== null
      ? keysOf(value as object, `${prefix}${key}.`)
      : [`${prefix}${key}`],
  );

describe("the two catalogues", () => {
  it("carry exactly the same keys", () => {
    const pt = keysOf(ptBR).sort();
    const en = keysOf(enUS).sort();

    expect({ missingInEnglish: pt.filter((k) => !en.includes(k)) }).toEqual({ missingInEnglish: [] });
    expect({ missingInPortuguese: en.filter((k) => !pt.includes(k)) }).toEqual({
      missingInPortuguese: [],
    });
  });

  it("has no key left empty, which reads as a missing screen rather than a missing word", () => {
    const empty = (catalogue: object, locale: string) =>
      keysOf(catalogue)
        .filter((key) => {
          const value = key.split(".").reduce<unknown>((o, k) => (o as Record<string, unknown>)?.[k], catalogue);
          return typeof value !== "string" || value.trim() === "";
        })
        .map((key) => `${locale}:${key}`);

    expect([...empty(ptBR, "pt-BR"), ...empty(enUS, "en-US")]).toEqual([]);
  });
});
