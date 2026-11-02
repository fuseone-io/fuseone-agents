import { useInfiniteQuery } from "@tanstack/react-query";
import { api, unwrap, type WorkItemTrail } from "@/lib/api/client";

export const workItemKeys = {
  all: ["work-items"] as const,
  trail: (ref: string) => [...workItemKeys.all, "trail", ref] as const,
};

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
