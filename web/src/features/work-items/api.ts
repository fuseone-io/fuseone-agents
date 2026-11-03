import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  api,
  unwrap,
  type WorkItemMeasures,
  type WorkItemTrail,
} from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";

export const workItemKeys = {
  all: ["work-items"] as const,
  trail: (ref: string) => [...workItemKeys.all, "trail", ref] as const,
  measures: (scope: string, query: MeasureQuery) =>
    [...workItemKeys.all, "measures", scope, query] as const,
};

/** Narrower than the active scope: one area of it, one agent. Empty is all. */
export interface MeasureFilters {
  area?: string;
  agentId?: string;
}

/** Exactly what is sent, which is also what the cache is keyed by. */
interface MeasureQuery extends MeasureFilters {
  weeks: number;
  company?: string;
}

/**
 * One card's trail, a page of runs at a time: the card's own runs and those
 * of the pull requests linked to it.
 *
 * The roles come with the first page only — the server folds them over every
 * run the caller may read — so they are read from there and never summed
 * across pages. Each page is narrowed by the server again; the cursor is only
 * where to resume.
 */
export function useWorkItemTrail(ref: string) {
  const query = useInfiniteQuery({
    queryKey: workItemKeys.trail(ref),
    enabled: ref !== "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/work-items/trail", {
          params: { query: { ref, cursor: pageParam, limit: 50 } },
        }),
      ) as WorkItemTrail,
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last: WorkItemTrail) => last.nextCursor ?? undefined,
  });
  const pages = query.data?.pages ?? [];
  return {
    ...query,
    runs: pages.flatMap((page) => page.runs),
    roles: pages[0]?.roles ?? [],
    // What the server resolved the reference to: the card itself, or — asked
    // by a pull request linked to it — the card, with the pull request asked.
    card: pages[0]?.ref,
    resolvedFrom: pages[0]?.resolvedFrom,
  };
}

/**
 * The ledger's weekly measures per role (#201), for the `weeks` whole weeks
 * ending with the current one.
 *
 * The active scope travels as it does everywhere, and it prevails: an area
 * picked here only narrows a scope that names none, and is never sent beside
 * an area the scope already chose. The rows come back as the server sent
 * them, oldest week first: the order they are shown in is the panel's.
 */
export function useWorkItemMeasures(weeks: number, filters: MeasureFilters) {
  const scope = useScopeFilter();
  const query = {
    weeks,
    company: scope.params.company,
    area: scope.params.area ?? (filters.area || undefined),
    agentId: filters.agentId || undefined,
  };
  return useQuery({
    queryKey: workItemKeys.measures(scope.key, query),
    queryFn: async () =>
      unwrap(
        await api.GET("/work-items/measures", { params: { query } }),
      ) as WorkItemMeasures,
  });
}
