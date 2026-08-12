import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";

export interface AuditFilters {
  since?: string;
  actor?: string;
  source?: "ledger" | "admin";
}

/**
 * The trail, newest first, across both records.
 *
 * One request rather than two merged here: the tenth page of a merge is not
 * the merge of two tenth pages, and an audit trail that silently drops entries
 * between pages is not one.
 */
export function useAudit(filters: AuditFilters) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: ["audit", scope.key, filters] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/audit", { params: { query: { ...scope.params, ...filters, limit: 100 } } }),
      ),
  });
}
