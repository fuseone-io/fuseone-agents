import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api, unwrap, type Phase } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import { usePagedQuery } from "@/features/runs/use-paged";
import { useSettled } from "@/hooks/use-settled";

// Query keys are centralised per feature so an invalidation cannot miss a
// cache entry because two call sites spelled the key differently.
export const runKeys = {
  all: ["runs"] as const,
  list: (scope: string, filters: RunFilters) =>
    [...runKeys.all, "list", scope, filters] as const,
  evidence: (company: string, area: string, search: string) =>
    [...runKeys.all, "evidence", company, area, search] as const,
  detail: (runId: string) => [...runKeys.all, "detail", runId] as const,
  steps: (runId: string) => [...runKeys.all, "steps", runId] as const,
  verify: (runId: string) => [...runKeys.all, "verify", runId] as const,
  content: (runId: string, seq: number) =>
    [...runKeys.all, "content", runId, seq] as const,
};

export interface RunFilters {
  phase?: Phase;
  agentId?: string;
  /** ISO instant; only runs started at or after it. */
  since?: string;
  /** ISO instant; only runs started before it. Needed to close a window that
   *  is being compared against another one. */
  until?: string;
  /** Matches the run or agent identifier, applied by the server. */
  q?: string;
}

export function useRuns(filters: RunFilters = {}) {
  const scope = useScopeFilter();
  return usePagedQuery(runKeys.list(scope.key, filters), async (cursor) =>
    unwrap(
      await api.GET("/runs", {
        params: {
          query: { ...scope.params, ...filters, limit: 50, cursor },
        },
      }),
    ),
  );
}

/**
 * Finished runs a person may cite while teaching memory.
 *
 * This takes the form's concrete scope rather than the page-wide scope: a
 * global viewer may be composing for one company and area, and showing runs
 * from every reachable scope would make a valid-looking choice fail later.
 * Search settles before reaching the server because prefixes are not choices.
 */
export function useEvidenceRuns({
  company,
  area,
  search,
  enabled = true,
}: {
  company: string;
  area: string;
  search: string;
  enabled?: boolean;
}) {
  const current = search.trim();
  const settled = useSettled(current, 300);
  const isSettling = current !== settled;
  const query = useQuery({
    queryKey: runKeys.evidence(company, area, settled),
    enabled: enabled && Boolean(company && area) && !isSettling,
    queryFn: async () =>
      unwrap(
        await api.GET("/runs", {
          params: {
            query: {
              company,
              area,
              phase: "finished",
              q: settled || undefined,
              limit: 25,
            },
          },
        }),
      ),
  });
  return {
    ...query,
    items: query.data?.items ?? [],
    hasMore: Boolean(query.data?.nextCursor),
    isSettling,
  };
}

export function useRun(runId: string, enabled = true) {
  return useQuery({
    queryKey: runKeys.detail(runId),
    enabled: enabled && Boolean(runId),
    queryFn: async () =>
      unwrap(await api.GET("/runs/{runId}", { params: { path: { runId } } })),
  });
}

/**
 * The run's trail, a page at a time.
 *
 * The trail is the audit record, so a page that stopped at two hundred steps
 * and said nothing was the worst of the truncations: a long run read as a
 * short one, and nothing on screen admitted it. The chain's own sequence is
 * the cursor here rather than an opaque token — for a hash chain an integer
 * position is both honest and useful.
 */
export function useRunSteps(runId: string) {
  const query = useInfiniteQuery({
    queryKey: runKeys.steps(runId),
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/runs/{runId}/steps", {
          params: {
            path: { runId },
            query: { limit: 200, fromSeq: pageParam as number | undefined },
          },
        }),
      ),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.nextSeq ?? undefined,
  });

  // The whole chain, fetched without being asked. Everything on the run screen
  // is derived from the trail — the cost, the diagram, the side rail, the
  // count of steps — so a trail stopped halfway is not a shorter page, it is a
  // set of wrong figures. The pages exist to bound one request, not to bound
  // what the reader gets.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = query;
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  return {
    ...query,
    items: (query.data?.pages ?? []).flatMap((page) => page.items),
    /** True until the last page has arrived, so a partial trail can say so. */
    isCompleting: query.hasNextPage || query.isFetchingNextPage,
  };
}

/**
 * The content a step references — proposed arguments, a tool's answer.
 *
 * A separate request from the trail on purpose: the trail is read constantly
 * and by many people, and what sits behind it routinely carries personal data.
 * It is fetched when somebody opens the step that needs it, which is why this
 * takes an `enabled` rather than being called for every row.
 */
export function useStepContent(
  runId: string,
  seq: number | undefined,
  enabled = true,
) {
  return useQuery({
    queryKey: runKeys.content(runId, seq ?? 0),
    enabled: enabled && seq !== undefined,
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/{runId}/steps/{seq}/content", {
          params: { path: { runId, seq: seq as number } },
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
    mutationFn: async (input: {
      approved: boolean;
      atSeq: number;
      note?: string;
    }) =>
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
  const scope = useScopeFilter();
  return useQuery({
    queryKey: [...runKeys.all, "stats", scope.key, filters] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/stats", {
          params: {
            query: {
              ...scope.params,
              agentId: filters.agentId,
              since: filters.since,
              until: filters.until,
            },
          },
        }),
      ),
  });
}
