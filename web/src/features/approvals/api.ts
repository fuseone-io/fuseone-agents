import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";

export const approvalKeys = {
  all: ["approvals"] as const,
  inbox: (scope: string) => [...approvalKeys.all, "inbox", scope] as const,
};

export function useApprovals() {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: approvalKeys.inbox(scope.key),
    queryFn: async () =>
      unwrap(await api.GET("/approvals", { params: { query: scope.params } })),
    // The inbox is what a manager keeps open; a short interval keeps it honest
    // without needing a live stream for a list this small.
    refetchInterval: 15_000,
  });
}
