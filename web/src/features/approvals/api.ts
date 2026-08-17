import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import { usePagedQuery } from "@/features/runs/use-paged";

export const approvalKeys = {
  all: ["approvals"] as const,
  inbox: (scope: string) => [...approvalKeys.all, "inbox", scope] as const,
};

export function useApprovals() {
  const scope = useScopeFilter();
  return usePagedQuery(
    approvalKeys.inbox(scope.key),
    async (cursor) =>
      unwrap(
        await api.GET("/approvals", {
          params: { query: { ...scope.params, limit: 50, cursor } },
        }),
      ),
    {
      // The inbox is what a manager keeps open; a short interval keeps it honest
      // without needing a live stream for a list this small.
      refetchInterval: 15_000,
    },
  );
}
