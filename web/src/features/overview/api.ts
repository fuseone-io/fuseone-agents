import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";

/**
 * The overview's reads.
 *
 * Every one is a separate query on purpose: the screen is six panels that
 * answer six questions, and a single endpoint shaped to fill this page would
 * have to change every time the page did.
 */
export const overviewKeys = {
  all: ["overview"] as const,
  throughput: (scope: string, since: string) =>
    [...overviewKeys.all, "throughput", scope, since] as const,
  decisions: (scope: string, since: string) =>
    [...overviewKeys.all, "decisions", scope, since] as const,
};

export function useThroughput(since: string) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: overviewKeys.throughput(scope.key, since),
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/throughput", {
          params: { query: { ...scope.params, since } },
        }),
      ),
  });
}

export function useDecisions(since: string, limit = 12) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: overviewKeys.decisions(scope.key, since),
    queryFn: async () =>
      unwrap(
        await api.GET("/decisions", {
          params: { query: { ...scope.params, since, limit } },
        }),
      ),
    // The Gate keeps deciding while somebody watches the feed. Short enough
    // to feel live, long enough not to be a load generator on a shared API.
    refetchInterval: 15_000,
  });
}
