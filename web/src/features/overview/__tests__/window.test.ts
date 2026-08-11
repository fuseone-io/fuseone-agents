import { describe, expect, it } from "vitest";
import { deltaOf, windowsFor } from "@/features/overview/window";

const NOON = new Date("2026-08-11T12:34:56").getTime();

describe("the overview's windows", () => {
  it("closes yesterday at the start of today, so the two never overlap", () => {
    // The whole point of the comparison. A yesterday with no upper bound
    // includes the today it is being compared to.
    const { current, previous } = windowsFor(NOON);
    expect(previous.until).toBe(current.since);
  });

  it("returns the same bounds for calls a moment apart", () => {
    // These are query keys. A bound that moves with the clock makes every
    // render a new key: refetch, re-render, refetch, until the tab closes.
    expect(windowsFor(NOON)).toEqual(windowsFor(NOON + 90_000));
  });

  it("spans exactly one day in each direction", () => {
    const { current, previous } = windowsFor(NOON);
    const day = 24 * 60 * 60 * 1000;
    expect(Date.parse(current.until) - Date.parse(current.since)).toBe(day);
    expect(Date.parse(previous.until) - Date.parse(previous.since)).toBe(day);
  });
});

describe("the delta between two figures", () => {
  it("reports the share of change", () => {
    expect(deltaOf(120, 100)).toBeCloseTo(0.2);
    expect(deltaOf(80, 100)).toBeCloseTo(-0.2);
  });

  it("has no answer when there is nothing to compare against", () => {
    // "+100%" from a base of zero is not a measurement, and a reader would
    // take it for one.
    expect(deltaOf(5, 0)).toBeUndefined();
  });
});
