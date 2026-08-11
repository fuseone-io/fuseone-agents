import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Phase } from "@/lib/api/client";

// Query keys are centralised per feature so an invalidation cannot miss a
// cache entry because two call sites spelled the key differently.
export const runKeys = {
  all: ["runs"] as const,
  list: (filters: RunFilters) => [...runKeys.all, "list", filters] as const,
  detail: (runId: string) => [...runKeys.all, "detail", runId] as const,
  steps: (runId: string) => [...runKeys.all, "steps", runId] as const,
  verify: (runId: string) => [...runKeys.all, "verify", runId] as const,
};

export interface RunFilters {
  phase?: Phase;
  agentId?: string;
  /** ISO instant; only runs started at or after it. */
  since?: string;
  /** Matches the run or agent identifier, applied by the server. */
  q?: string;
}

export function useRuns(filters: RunFilters = {}) {
  return useQuery({
    queryKey: runKeys.list(filters),
    queryFn: async () =>
      unwrap(
        await api.GET("/runs", {
          params: { query: { ...filters, limit: 50 } },
        }),
      ),
  });
}

export function useRun(runId: string) {
  return useQuery({
    queryKey: runKeys.detail(runId),
    queryFn: async () =>
      unwrap(await api.GET("/runs/{runId}", { params: { path: { runId } } })),
  });
}

export function useRunSteps(runId: string) {
  return useQuery({
    queryKey: runKeys.steps(runId),
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/{runId}/steps", {
          params: { path: { runId }, query: { limit: 200 } },
        }),
      ),
  });
}

/**
 * Chain verification is deliberately not part of the run query: it walks every
 * step, so it runs when someone asks for it rather than on every page view.
 */
export function useVerifyRun(runId: string) {
  return useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/runs/{runId}/verify", {
          params: { path: { runId } },
        }),
      ),
  });
}

export function useDecideApproval(runId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { approved: boolean; atSeq: number; note?: string }) =>
      unwrap(
        await api.POST("/runs/{runId}/approvals", {
          params: { path: { runId } },
          body: input,
        }),
      ),
    onSuccess: () => {
      // The decision appends a step, so the trail and every list that counts
      // pending approvals are both stale.
      void queryClient.invalidateQueries({ queryKey: runKeys.all });
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
    },
  });
}

/**
 * Aggregates over every matching run, not over the page.
 *
 * A separate query on purpose: the list is paginated and the counts are not,
 * so tying them together would make the figures depend on how many rows the
 * table happened to ask for.
 */
export function useRunStats(filters: RunFilters = {}) {
  return useQuery({
    queryKey: [...runKeys.all, "stats", filters] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/stats", {
          params: { query: { agentId: filters.agentId, since: filters.since } },
        }),
      ),
  });
}
