import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import ptBR from "@/i18n/pt-BR.json";

/**
 * Keys held in a map, not written inside a t() call.
 *
 * The sibling test scans for keys written inside a t() call and cannot see one
 * that reaches t() through a variable — `PHASE_LABELS[phase]`, a Record of step kinds to
 * titles, a label prop. Those are exactly where three keys were found
 * rendering their own names in the middle of the interface: an approver read
 * "policies.rule" where the word "Regra" belonged.
 *
 * Anything shaped like a key and starting with a real namespace is treated as
 * one. A string that looks that much like a key and resolves to nothing is a
 * bug whichever way it was going to be used.
 */
const NAMESPACES = new Set(Object.keys(ptBR));

/**
 * Dotted literals that are identifiers, not keys.
 *
 * Audit actions and policy field paths happen to be shaped like keys, and
 * their namespaces collide with real ones. Listing them here rather than
 * loosening the pattern keeps the guard sharp: a new dotted literal has to
 * either resolve to a translation or be declared, deliberately, as a name.
 */
const IDENTIFIERS = new Set([
  // Audit action names, as the server records them.
  "gate.allowed",
  "gate.constrained",
  "gate.escalated",
  "gate.blocked",
  "gate.decided",
  // Policy condition field paths.
  "scope.area",
]);

const files = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    return statSync(path).isDirectory()
      ? files(path)
      : path.match(/\.tsx?$/) && !path.includes("__tests__")
        ? [path]
        : [];
  });

const at = (key: string): unknown =>
  key
    .split(".")
    .reduce<unknown>((o, k) => (o as Record<string, unknown>)?.[k], ptBR);

const has = (key: string): boolean =>
  at(key) !== undefined || at(`${key}_one`) !== undefined;

describe("a key held in a map", () => {
  it("resolves to a translation", () => {
    const found = new Set<string>();
    for (const file of files("src")) {
      for (const match of readFileSync(file, "utf8").matchAll(
        /"([a-z][A-Za-z]*)\.([A-Za-z][\w.]*)"/g,
      )) {
        const [literal, namespace] = [match[0].slice(1, -1), match[1] ?? ""];
        if (NAMESPACES.has(namespace) && !IDENTIFIERS.has(literal)) {
          found.add(literal);
        }
      }
    }

    expect({
      found: found.size,
      missing: [...found].filter((k) => !has(k)).sort(),
    }).toMatchObject({ missing: [] });
  });
});
