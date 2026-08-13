import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type BudgetAlert = components["schemas"]["BudgetAlert"];

/**
 * Which scopes are running out of money (PRD FO-05).
 *
 * One entry per scope, naming the highest threshold reached this period. An
 * empty list means nothing has crossed half.
 */
export function useBudgetAlerts(scope?: { company?: string; area?: string }) {
  return useQuery({
    queryKey: ["budget-alerts", scope?.company ?? "", scope?.area ?? ""],
    queryFn: async () =>
      unwrap(
        await api.GET("/budgets/alerts", {
          params: { query: { company: scope?.company, area: scope?.area } },
        }),
      ).items,
    refetchInterval: 60_000,
  });
}
