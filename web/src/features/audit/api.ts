import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";

export interface AuditFilters {
  since?: string;
  actor?: string;
  source?: "ledger" | "admin";
}

/**
 * One page of the trail, newest first, across both records.
 *
 * Cursor-based, not offset-based. The page number on screen is only the
 * operator's local position in this reading session; the cursor is what names
 * the database boundary. Offset pagination over a live append-only trail repeats
 * and skips rows as new entries arrive.
 */
export function useAuditPage(filters: AuditFilters, cursor?: string) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: ["audit", scope.key, filters, cursor] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/audit", {
          params: { query: { ...scope.params, ...filters, limit: 50, cursor } },
        }),
      ),
  });
}
