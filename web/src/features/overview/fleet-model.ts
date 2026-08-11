import type { ThroughputBucket } from "@/lib/api/client";

/**
 * Each agent's runs by the hour, from the buckets the overview already has.
 *
 * Derived from the same rows as the chart rather than fetched per agent: one
 * request per card would be a dozen requests to draw a dozen small lines, and
 * they could disagree with each other.
 */
export function trendsByAgent(
  buckets: ThroughputBucket[],
  since: string,
  hours = 24,
): Map<string, number[]> {
  const start = Date.parse(since);
  const trends = new Map<string, number[]>();

  const at = (bucket: ThroughputBucket) =>
    Math.round((Date.parse(bucket.at) - start) / 3_600_000);

  for (const bucket of buckets) {
    const hour = at(bucket);
    if (hour < 0 || hour >= hours) continue;

    for (const [agent, count] of Object.entries(bucket.byAgent ?? {})) {
      const series = trends.get(agent) ?? new Array<number>(hours).fill(0);
      series[hour] = count;
      trends.set(agent, series);
    }
  }
  return trends;
}
