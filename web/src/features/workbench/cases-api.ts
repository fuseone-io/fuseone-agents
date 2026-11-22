import { useQuery } from "@tanstack/react-query";
import { regressionKeys } from "@/features/agents/regressions-api";
import { isAbsent } from "@/features/workbench/api";
import { api, unwrap } from "@/lib/api/client";
import type { RegressionCase } from "@/features/agents/regressions-api";

/*
The corpus as the workbench reads it (NT-018 phase 5b). An agent with no corpus within the
person's reach answers 404, the same as one with no corpus at all, and both read here as no
cases yet: the tab says how one is made. Keyed under the corpus, so keeping or removing a
case reads it again.
*/

export function useCases(agentId: string) {
  return useQuery({
    queryKey: [...regressionKeys.of(agentId), "workbench"] as const,
    queryFn: async (): Promise<RegressionCase[]> => {
      try {
        return unwrap(await api.GET("/agents/{agentId}/regressions", { params: { path: { agentId } } })).items;
      } catch (error) {
        if (isAbsent(error)) return [];
        throw error;
      }
    },
    enabled: agentId !== "",
    retry: false,
  });
}
