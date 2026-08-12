import { describe, expect, it } from "vitest";
import { trendsByAgent } from "@/features/overview/fleet-model";
import type { ThroughputBucket } from "@/lib/api/client";

const SINCE = "2026-08-11T00:00:00.000Z";

function bucket(
  hour: number,
  byAgent: Record<string, number>,
): ThroughputBucket {
  return {
    at: new Date(Date.parse(SINCE) + hour * 3_600_000).toISOString(),
    byAgent,
    byPhase: {},
    total: Object.values(byAgent).reduce((a, b) => a + b, 0),
    micros: 0,
  };
}

describe("each agent's trend", () => {
  it("gives every agent a full day, so two sparklines share a time axis", () => {
    // Drawn side by side. Series of different lengths would put the same hour
    // at different x positions on adjacent cards.
    const trends = trendsByAgent(
      [bucket(9, { triage: 3 }), bucket(11, { billing: 1 })],
      SINCE,
    );

    expect(trends.get("triage")).toHaveLength(24);
    expect(trends.get("billing")).toHaveLength(24);
  });

  it("puts each hour's count at that hour", () => {
    const trends = trendsByAgent([bucket(9, { triage: 3 })], SINCE);

    expect(trends.get("triage")?.[9]).toBe(3);
    expect(trends.get("triage")?.[8]).toBe(0);
  });

  it("ignores hours outside the day it was asked for", () => {
    // The server bounds the window, but a clock skew or a stale cache can
    // still deliver one, and writing past the end of the array would throw.
    const trends = trendsByAgent([bucket(30, { triage: 2 })], SINCE);

    expect(trends.size).toBe(0);
  });
});
