import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import ptBR from "@/i18n/pt-BR.json";

/**
 * Every key a component asks for has to exist.
 *
 * A missing key renders its own name — "agents.idempotencyKey" in the middle
 * of a sentence — and the parity test cannot see it, because a key absent from
 * both catalogues is perfectly in parity.
 */
const files = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const path = join(dir, entry);
    return statSync(path).isDirectory()
      ? files(path)
      : path.match(/\.tsx?$/) && !path.includes("__tests__")
        ? // A test's own prose is not the interface. One describing this
          // scanner tripped it by quoting the pattern it looks for.
          [path]
        : [];
  });

const at = (key: string): unknown =>
  key
    .split(".")
    .reduce<unknown>((o, k) => (o as Record<string, unknown>)?.[k], ptBR);

/** A counted key is stored under its plural forms, and asked for without one. */
const has = (key: string): boolean =>
  at(key) !== undefined || at(`${key}_one`) !== undefined;

describe("every key a component asks for", () => {
  it("exists in the catalogue", () => {
    const asked = new Set<string>();
    for (const file of files("src")) {
      const source = readFileSync(file, "utf8");
      for (const match of source.matchAll(/\bt\(\s*"([a-z][\w.]+)"/g))
        asked.add(match[1] ?? "");
    }

    expect({
      asked: asked.size,
      missing: [...asked].filter((k) => !has(k)).sort(),
    }).toMatchObject({
      missing: [],
    });
  });
});
