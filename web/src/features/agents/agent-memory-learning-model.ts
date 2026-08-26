import type { MemoryLearningPolicy } from "@/lib/api/client";

export type LearningMode = NonNullable<MemoryLearningPolicy["mode"]>;

export const DEFAULT_MIN_OBSERVATIONS = 3;
export const DEFAULT_TTL_DAYS = 30;
export const MAX_MIN_OBSERVATIONS = 8;
export const MAX_TTL_DAYS = 365;

export function normaliseLearning(
  policy?: MemoryLearningPolicy,
): Required<MemoryLearningPolicy> {
  if (!policy?.mode || policy.mode === "off") {
    return {
      mode: "off",
      minObservations: DEFAULT_MIN_OBSERVATIONS,
      ttlDays: DEFAULT_TTL_DAYS,
    };
  }
  return {
    mode: policy.mode,
    minObservations: boundedInt(policy.minObservations, 2, MAX_MIN_OBSERVATIONS),
    ttlDays: boundedInt(policy.ttlDays, 1, MAX_TTL_DAYS),
  };
}

export function boundedInt(value: unknown, min: number, max: number): number {
  const n = Number(value);
  if (!Number.isFinite(n)) return min;
  return Math.min(max, Math.max(min, Math.trunc(n)));
}
