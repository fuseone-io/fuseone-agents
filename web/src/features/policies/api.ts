import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { PolicyInput } from "@/lib/api/client";

export const policyKeys = {
  all: ["policies"] as const,
  list: (since?: string) => [...policyKeys.all, "list", since ?? "default"] as const,
};

/** The rules in force, with how often each one decided. */
export function usePolicies(since?: string) {
  return useQuery({
    queryKey: policyKeys.list(since),
    queryFn: async () => unwrap(await api.GET("/policies", { params: { query: { since } } })),
  });
}

/**
 * Writes one rule.
 *
 * Creating and editing are the same call: the code is the identity, set once
 * and never changed, because it appears in the trail and in the message
 * somebody denied reads.
 */
export function usePutPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ code, policy }: { code: string; policy: PolicyInput }) =>
      unwrap(await api.PUT("/policies/{code}", { params: { path: { code } }, body: policy })),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: policyKeys.all }),
  });
}

export function useDeletePolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (code: string) =>
      unwrap(await api.DELETE("/policies/{code}", { params: { path: { code } } })),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: policyKeys.all }),
  });
}
