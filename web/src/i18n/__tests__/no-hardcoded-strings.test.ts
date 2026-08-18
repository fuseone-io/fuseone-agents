import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * No user-visible string is written into a component.
 *
 * Language-blind on purpose: a hardcoded English label is exactly as wrong as
 * a hardcoded Portuguese one. Neither can be translated, and what a reader
 * actually sees is a card whose figures read RUNS, FINISHED, COST — and then
 * GATILHOS.
 *
 * Three positions, which are the three that reached a screen: a label-ish prop
 * given a bare string, text sitting between tags, and a template literal with
 * Portuguese in it. The last one is here because it is where the worst of them
 * hid — a module with no React context cannot translate, so it returned
 * "gatilho cron" and the English trail rendered it verbatim.
 */
const SKIP = [
  "i18n/",
  "__tests__",
  // CLI-owned primitives. Their strings are shadcn's, and a translation here
  // would be undone by the next `shadcn add`.
  "components/ui/",
  "lib/api/schema.gen",
  /*
  The block labels an instruction is written with.

  They are not interface text: they are written into the payload the model
  receives, and they are read back out of it when somebody opens the version
  again. Putting them in the catalogue would make what a definition *is*
  depend on the language of whoever last opened the console — a colleague
  reading in English would find a Portuguese author's blocks collapsed into an
  unlabelled paragraph, and saving would rewrite the text.

  So both languages live in that module, and it recognises either on the way
  back in. It is the one place where a Portuguese string in a source file is
  the correct answer.
  */
  "agents/instruction-blocks",
];

const PROPS =
  "(?:label|title|placeholder|hint|description|alt|aria-label|emptyLabel)";

/**
 * Sample values, not prose: an identifier, a path, a URL, a brand.
 *
 * Listed rather than pattern-matched, so a new one is a decision somebody
 * makes on purpose. Translating `openai` would be worse than leaving it.
 */
const SAMPLES = new Set([
  "FuseOne Agents",
  "Fuse",
  "One",
  "Agents",
  "Anthropic",
  "anthropic",
  "openai",
  "default",
  "low",
  "suporte",
  "run_suporte_1786...",
  // A Slack channel id and the name of a channel: both are shown as the
  // shape of the thing being asked for, and translating either would be
  // teaching somebody the wrong format.
  "C0123ABCDEF",
  "U0123ABCDEF",
  // An example identifier, shown as the shape of the thing being asked for.
  "acme",
  "#alertas",
  "https://api.example.com/mcp/",
  "https://api.openai.com/v1",
  "/usr/local/bin/crm-mcp",
  "--config /etc/crm.yaml",
]);

/**
 * Portuguese orthography, or a word that only occurs in Portuguese prose.
 *
 * Deliberately not a general language detector. It has to catch what a
 * reviewer would catch by glancing, and no more, or it becomes a test people
 * silence.
 */
const PORTUGUESE =
  /[ãõçáéíóúâêôà]|\b(em curso|paradas|não|uma|para|com|sem|que|este|esta|isso|aqui|onde|quando|todos|nada|ainda|foi|será|está|estão|têm|pelo|pela|dos|das|nos|nas)\b/i;

const NOT_PROSE = new RegExp(
  "^[a-z][\\w]*([./:@#-][\\w*]+)+$" + // crm.reply, /etc/x
    "|^[A-Z_]+$" + // SCREAMING_CASE
    "|^[\\d\\s.,:%$R—–-]+$" + // numbers and separators
    "|^\\{\\{.*\\}\\}$",
);

const files = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    if (SKIP.some((s) => path.includes(s))) return [];
    return statSync(path).isDirectory()
      ? files(path)
      : path.match(/\.tsx?$/)
        ? [path]
        : [];
  });

const prose = (text: string): boolean => {
  const t = text.trim();
  if (t.length < 2 || SAMPLES.has(t) || NOT_PROSE.test(t)) return false;
  // An i18n key is handled by the sibling scanners.
  if (/^[a-z][\w]*(\.[\w]+)+$/.test(t)) return false;
  return /[A-Za-zÀ-ú]{2,}/.test(t);
};

describe("a string a person reads", () => {
  it("is never written into a component", () => {
    const found: string[] = [];
    for (const file of files("src")) {
      const source = readFileSync(file, "utf8");
      source.split("\n").forEach((line, i) => {
        const trimmed = line.trim();
        if (trimmed.startsWith("//") || trimmed.startsWith("*")) return;

        for (const m of line.matchAll(
          new RegExp(`${PROPS}=(?:"([^"]{2,})"|\\{"([^"]{2,})"\\})`, "g"),
        )) {
          const text = m[1] ?? m[2] ?? "";
          if (prose(text)) found.push(`${file}:${i + 1} ${text}`);
        }
        // Text between tags, and only where tags exist. A .ts module has no
        // JSX, so the same pattern there reads a generic — `=> Promise<Page<T>>`
        // is not a label somebody sees.
        if (file.endsWith(".tsx")) {
          for (const m of line.matchAll(
            />\s*([A-Za-zÀ-ú][^<>{}\n]{2,})\s*</g,
          )) {
            if (prose(m[1] ?? "")) found.push(`${file}:${i + 1} ${m[1]}`);
          }
        }

        // Portuguese anywhere at all, including inside a template literal.
        // Only Portuguese here: an English literal in an expression is usually
        // a key, an id or a class, and flagging every one would make the test
        // an allowlist rather than a guard.
        for (const m of line.matchAll(/[`"']([^`"'\n]{4,})[`"']/g)) {
          const text = m[1] ?? "";
          if (!SAMPLES.has(text) && PORTUGUESE.test(text)) {
            found.push(`${file}:${i + 1} ${text}`);
          }
        }
      });
    }

    expect(found).toEqual([]);
  });
});
