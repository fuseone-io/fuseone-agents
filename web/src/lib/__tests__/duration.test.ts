import { describe, expect, it } from "vitest";
import { formatDuration } from "@/lib/format";

const START = "2026-08-11T12:00:00Z";
const at = (seconds: number) =>
  new Date(Date.parse(START) + seconds * 1000).toISOString();

describe("run duration", () => {
  it("reads coarsely, because the reader is comparing a column not timing a lap", () => {
    expect(formatDuration(START, at(161))).toBe("2m 41s");
    expect(formatDuration(START, at(45))).toBe("45s");
    expect(formatDuration(START, at(3900))).toBe("1h 5m");
  });

  it("measures an unfinished run against now, so a running row is not blank", () => {
    expect(formatDuration(START, null, Date.parse(START) + 30_000)).toBe("30s");
  });

  it("says so rather than printing NaN when an instant is unusable", () => {
    expect(formatDuration("not a date", null)).toBe("—");
  });
});
