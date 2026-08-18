import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useScopeFilter } from "@/features/scope/use-scope-filter";

export const runtimeKeys = {
  all: ["runtime"] as const,
  health: (scope: string, since: string) =>
    [...runtimeKeys.all, "health", scope, since] as const,
};

export function useRuntimeHealth(since: string) {
  const scope = useScopeFilter();
  return useQuery({
    queryKey: runtimeKeys.health(scope.key, since),
    queryFn: async () =>
      unwrap(
        await api.GET("/runtime", {
          params: { query: { ...scope.params, since } },
        }),
      ),
  });
}
