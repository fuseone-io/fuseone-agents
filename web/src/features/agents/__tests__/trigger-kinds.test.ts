import { describe, expect, it } from "vitest";
import ptBR from "@/i18n/pt-BR.json";
import enUS from "@/i18n/en-US.json";
import { TRIGGER_KINDS } from "@/features/agents/trigger-kinds";

/**
 * Every trigger kind has the four strings the screen asks it for.
 *
 * The sibling guard cannot see these: they are built as `agents.trigger.${kind}`
 * and a scanner reading source text finds a template literal, not a key. The
 * gap is not hypothetical — the section shipped with the kinds renamed and the
 * strings not, and what a reader saw on the button was the literal text
 * `agents.trigger.cron`.
 *
 * So the list of kinds is the test: add one and this fails until it can be
 * read in both languages.
 */
type Catalogue = { agents: Record<string, Record<string, string>> };

const CATALOGUES: Record<string, Catalogue> = {
  "pt-BR": ptBR as unknown as Catalogue,
  "en-US": enUS as unknown as Catalogue,
};

describe("what a trigger kind is called", () => {
  it("is written in both languages, for every kind", () => {
    const missing: string[] = [];

    for (const [locale, catalogue] of Object.entries(CATALOGUES)) {
      for (const kind of TRIGGER_KINDS) {
        for (const group of [
          "trigger",
          "triggerField",
          "triggerExample",
          "triggerNeeds",
        ]) {
          if (!catalogue.agents[group]?.[kind]) {
            missing.push(`${locale}: agents.${group}.${kind}`);
          }
        }
      }
    }

    expect(missing).toEqual([]);
  });
});
