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

/**
 * Creates somebody who signs in with a password.
 *
 * Beside the identity provider, never instead of it. Where a customer has
 * one, that is how people arrive — this is for the installation that has no
 * provider yet, which every installation is on its first day.
 */
export function useCreateLocalPerson() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      username: string;
      password: string;
      display?: string;
      email?: string;
    }) => unwrap(await api.POST("/admin/people/local", { body: input })),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: peopleKeys.all }),
  });
}

/** Sets or replaces somebody's password, and their handle when they have none. */
export function useSetPassword() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      principalId: string;
      password: string;
      username?: string;
    }) =>
      unwrap(
        await api.PUT("/admin/people/{principalId}/password", {
          params: { path: { principalId: input.principalId } },
          body: { password: input.password, username: input.username },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: peopleKeys.all }),
  });
}
