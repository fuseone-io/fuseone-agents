import { describe, expect, it } from "vitest";
import { normaliseLearning } from "@/features/agents/agent-memory-learning-model";

describe("agent memory learning policy", () => {
  it("defaults to off without deleting the review defaults", () => {
    expect(normaliseLearning()).toEqual({
      mode: "off",
      minObservations: 3,
      ttlDays: 30,
    });
  });

  it("bounds auto-confirm thresholds before they reach the API", () => {
    expect(
      normaliseLearning({
        mode: "auto_confirm",
        minObservations: 99,
        ttlDays: 10_000,
      }),
    ).toEqual({
      mode: "auto_confirm",
      minObservations: 8,
      ttlDays: 365,
    });
  });
});
