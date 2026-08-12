import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type Person = components["schemas"]["Person"];
export type HeldGrant = components["schemas"]["HeldGrant"];
export type GrantInput = components["schemas"]["GrantInput"];

export const peopleKeys = {
  all: ["people"] as const,
};

/** Everybody the installation knows about, and what each one holds. */
export function usePeople() {
  return useQuery({
    queryKey: peopleKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/people")),
  });
}

/**
 * Replaces the grants somebody was given directly.
 *
 * Only those: what an identity provider asserts is re-derived on every
 * sign-in, and this never touches it.
 */
export function useSetGrants() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { principalId: string; grants: GrantInput[] }) =>
      unwrap(
        await api.PUT("/admin/people/{principalId}/grants", {
          params: { path: { principalId: input.principalId } },
          body: { grants: input.grants },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: peopleKeys.all }),
  });
}
