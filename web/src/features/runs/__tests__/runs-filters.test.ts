import { describe, expect, it } from "vitest";
import { sinceFor } from "@/features/runs/runs-filters";

const NOW = Date.parse("2026-08-11T12:00:00Z");

describe("the period filter", () => {
  it("becomes the instant the server filters on, so a page is the whole answer", () => {
    expect(sinceFor("1", NOW)).toBe("2026-08-10T12:00:00.000Z");
    expect(sinceFor("30", NOW)).toBe("2026-07-12T12:00:00.000Z");
  });

  it("sends no bound at all for the whole period, rather than a date far in the past", () => {
    expect(sinceFor("all", NOW)).toBeUndefined();
  });
});

describe("the period filter as a query key", () => {
  it("returns the same instant for calls a moment apart", () => {
    // The result is part of a TanStack Query key. A value that moves with the
    // clock makes every render a new key: refetch, re-render, refetch, for as
    // long as the page is open.
    const first = sinceFor("7", NOW + 1);
    const second = sinceFor("7", NOW + 1520);
    expect(first).toBe(second);
  });
});
