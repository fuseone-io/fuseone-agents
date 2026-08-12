import { describe, expect, it } from "vitest";
import { ceilingOf, columnsFor } from "@/features/overview/throughput-model";
import type { ThroughputBucket } from "@/lib/api/client";

const SINCE = "2026-08-11T00:00:00.000Z";

function bucket(
  hour: number,
  byPhase: Record<string, number>,
): ThroughputBucket {
  const at = new Date(Date.parse(SINCE) + hour * 3_600_000).toISOString();
  return {
    at,
    byPhase,
    byAgent: {},
    micros: 0,
    total: Object.values(byPhase).reduce((a, b) => a + b, 0),
  };
}

describe("the throughput columns", () => {
  it("fills the hours the store had nothing to report", () => {
    // The store answers what it measured. A quiet morning would otherwise
    // read as a narrower day rather than an emptier one.
    const columns = columnsFor([bucket(9, { finished: 3 })], SINCE);

    expect(columns).toHaveLength(24);
    expect(columns[0]?.total).toBe(0);
    expect(columns[9]?.total).toBe(3);
  });

  it("folds a run still going in with the ones waiting", () => {
    // Three columns: finished, still going, stopped. A reader deciding
    // whether to act does not need the interpreter's phase.
    const columns = columnsFor(
      [bucket(1, { running: 2, awaiting_approval: 1 })],
      SINCE,
    );

    expect(columns[1]?.byState.waiting).toBe(3);
    expect(columns[1]?.byState.running).toBe(0);
  });

  it("counts a parked run as blocked, which is what a reader would act on", () => {
    const columns = columnsFor([bucket(2, { parked: 1, finished: 4 })], SINCE);

    expect(columns[2]?.byState.blocked).toBe(1);
    expect(columns[2]?.byState.done).toBe(4);
  });
});

describe("the axis ceiling", () => {
  it("rounds up so the tallest bar does not touch the top", () => {
    const columns = columnsFor([bucket(0, { finished: 5 })], SINCE);
    expect(ceilingOf(columns)).toBe(8);
  });

  it("keeps a scale on an empty day, rather than collapsing to nothing", () => {
    // A blank rectangle reads as broken. A quiet day reads as quiet.
    expect(ceilingOf(columnsFor([], SINCE))).toBeGreaterThan(0);
  });
});
