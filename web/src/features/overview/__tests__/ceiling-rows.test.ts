import { describe, expect, it } from "vitest";
import { ceilingRows } from "@/features/overview/ceiling-rows";
import type { ScopeBudget } from "@/features/admin/api";

const budget = (kind: ScopeBudget["scopeKind"], company: string, area: string, micros: number) =>
  ({ scopeKind: kind, scope: { company, area }, micros, period: "daily", enabled: true }) as ScopeBudget;

const spend = {
  byCompany: new Map([["cx", 40], ["devops", 10]]),
  byArea: new Map([["cx", 7]]),
  total: 50,
};

describe("pairing a ceiling with what was spent under it", () => {
  it("reads a company ceiling against the company's spend, not the area's", () => {
    const [row] = ceilingRows([budget("company", "cx", "", 100)], spend);
    expect(row).toMatchObject({ cap: 100, spent: 40 });
  });

  it("reads an area ceiling against the area's spend", () => {
    const [row] = ceilingRows([budget("area", "acme", "cx", 100)], spend);
    expect(row).toMatchObject({ cap: 100, spent: 7 });
  });

  it("reads an installation ceiling against everything spent", () => {
    const [row] = ceilingRows([budget("installation", "", "", 100)], spend);
    expect(row).toMatchObject({ cap: 100, spent: 50 });
  });

  it("gives ceilings on different scopes different keys, so none is dropped from the list", () => {
    const rows = ceilingRows(
      [budget("company", "cx", "", 1), budget("company", "devops", "", 2), budget("installation", "", "", 3)],
      spend,
    );
    expect(new Set(rows.map((r) => r.key)).size).toBe(3);
  });
});
