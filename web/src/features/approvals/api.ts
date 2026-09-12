import { api, unwrap } from "@/lib/api/client";
import { useQuery } from "@tanstack/react-query";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import { usePagedQuery } from "@/features/runs/use-paged";

export const approvalKeys = {
  all: ["approvals"] as const,
  inbox: (scope: string) => [...approvalKeys.all, "inbox", scope] as const,
  evidence: (runId: string, atSeq: number) =>
    [...approvalKeys.all, "evidence", runId, atSeq] as const,
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

export function useApprovalEvidence(runId: string, atSeq: number) {
  return useQuery({
    queryKey: approvalKeys.evidence(runId, atSeq),
    queryFn: async () =>
      unwrap(
        await api.GET("/runs/{runId}/approvals/{atSeq}/evidence", {
          params: { path: { runId, atSeq } },
        }),
      ),
    // Evidence is immutable for one approval step. A later question has a
    // different atSeq and therefore a different key.
    staleTime: Infinity,
  });
}
