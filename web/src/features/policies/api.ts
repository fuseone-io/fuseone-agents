import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { PolicyInput } from "@/lib/api/client";
import { ifMatch, revised, revisionReads } from "@/lib/api/revision";

export const policyKeys = {
  all: ["policies"] as const,
  list: (since?: string) =>
    [...policyKeys.all, "list", since ?? "default"] as const,
  detail: (code: string) => [...policyKeys.all, "detail", code] as const,
};

/** One rule as it is stored now, with the revision a write over it answers. */
export async function readPolicy(code: string) {
  return revised(
    await api.GET("/policies/{code}", { params: { path: { code } } }),
  );
}

/**
 * One rule, read by its code when its editor opens.
 *
 * Read again on every mount: the form takes its values once, and seeding it
 * from a cached copy would save somebody's edit over a version they never saw.
 */
export function usePolicy(code: string | undefined) {
  return useQuery({
    queryKey: policyKeys.detail(code ?? ""),
    queryFn: () => readPolicy(code!),
    enabled: Boolean(code),
    ...revisionReads,
  });
}

/** The rules in force, with how often each one decided. */
export function usePolicies(since?: string) {
  return useQuery({
    queryKey: policyKeys.list(since),
    queryFn: async () =>
      unwrap(await api.GET("/policies", { params: { query: { since } } })),
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
    mutationFn: async ({
      code,
      policy,
      etag,
    }: {
      code: string;
      policy: PolicyInput;
      /** The revision the editor read; a new rule has none. */
      etag?: string;
    }) =>
      unwrap(
        await api.PUT("/policies/{code}", {
          params: { path: { code }, header: ifMatch(etag) },
          body: policy,
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: policyKeys.all }),
  });
}

/**
 * What a draft rule would have done, against decisions already recorded.
 *
 * Runs on demand rather than on every keystroke: it reads the trail, and a
 * panel that re-queried while somebody typed a threshold would make the cost
 * of writing a rule proportional to how carefully they wrote it.
 */
export function useSimulatePolicy() {
  return useMutation({
    mutationFn: async (policy: PolicyInput) =>
      unwrap(await api.POST("/policies/simulate", { body: policy })),
  });
}

export function useDeletePolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ code, etag }: { code: string; etag: string }) =>
      unwrap(
        await api.DELETE("/policies/{code}", {
          params: { path: { code }, header: ifMatch(etag) },
        }),
      ),
    // Returning the promise keeps the mutation pending until active policy
    // lists have refetched, so the removed row cannot linger as a finished act.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: policyKeys.all }),
    // Refused as stale, the row on screen is what changed: show it again.
    onError: () =>
      void queryClient.invalidateQueries({ queryKey: policyKeys.all }),
  });
}
