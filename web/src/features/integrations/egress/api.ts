import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type EgressPolicy = components["schemas"]["MCPEgressPolicy"];

export const egressKeys = {
  all: ["integrations", "mcp-egress-policies"] as const,
};

/** Every stored policy: the platform's to read and write (NT-019). */
export function useEgressPolicies() {
  return useQuery({
    queryKey: egressKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/integrations/mcp-egress-policies", {})),
  });
}

/** Replaces a scope's list. An empty one is a policy too: it allows nothing. */
export function usePutEgressPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { company: string; area: string; origins: string[] }) =>
      unwrap(
        await api.PUT("/admin/integrations/mcp-egress-policies", {
          body: { company: input.company, ...(input.area ? { area: input.area } : {}), origins: input.origins },
        }),
      ),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: egressKeys.all }),
  });
}

/** Removes a scope's list: an area inherits its company's again, or reaches nothing. */
export function useDeleteEgressPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scope: { company: string; area?: string }) =>
      unwrap(
        await api.DELETE("/admin/integrations/mcp-egress-policies", {
          params: { query: { company: scope.company, ...(scope.area ? { area: scope.area } : {}) } },
        }),
      ),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: egressKeys.all }),
  });
}

/** One origin per line, blank lines and the spaces around each dropped. */
export function originsOf(text: string): string[] {
  return text.split("\n").map((line) => line.trim()).filter((line) => line !== "");
}

export function scopeLabel(policy: { company: string; area?: string }): string {
  return policy.area ? `${policy.company}/${policy.area}` : policy.company;
}
