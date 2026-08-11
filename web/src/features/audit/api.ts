import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

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
  return useQuery({
    queryKey: ["audit", filters] as const,
    queryFn: async () =>
      unwrap(await api.GET("/audit", { params: { query: { ...filters, limit: 100 } } })),
  });
}
