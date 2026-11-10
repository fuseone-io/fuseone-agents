import type { ExperimentListing } from "@/features/experiments/experiments-api";

/** The two picked, the one that opened first first. */
export function inOpeningOrder(
  experiments: ExperimentListing[],
  a: string,
  b: string,
): [string, string] {
  const opened = (id: string) =>
    Date.parse(experiments.find((e) => e.id === id)?.openedAt ?? "");
  return opened(a) <= opened(b) ? [a, b] : [b, a];
}
