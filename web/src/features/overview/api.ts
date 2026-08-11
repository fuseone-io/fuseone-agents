import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

/**
 * The overview's reads.
 *
 * Every one is a separate query on purpose: the screen is six panels that
 * answer six questions, and a single endpoint shaped to fill this page would
 * have to change every time the page did.
 */
export const overviewKeys = {
  all: ["overview"] as const,
  throughput: (since: string) => [...overviewKeys.all, "throughput", since] as const,
  decisions: (since: string) => [...overviewKeys.all, "decisions", since] as const,
};

export function useThroughput(since: string) {
  return useQuery({
    queryKey: overviewKeys.throughput(since),
    queryFn: async () =>
      unwrap(await api.GET("/runs/throughput", { params: { query: { since } } })),
  });
}

export function useDecisions(since: string, limit = 12) {
  return useQuery({
    queryKey: overviewKeys.decisions(since),
    queryFn: async () =>
      unwrap(await api.GET("/decisions", { params: { query: { since, limit } } })),
    // The Gate keeps deciding while somebody watches the feed. Short enough
    // to feel live, long enough not to be a load generator on a shared API.
    refetchInterval: 15_000,
  });
}
