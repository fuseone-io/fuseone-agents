import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type IdentityProvider = components["schemas"]["IdentityProvider"];
export type GroupMapping = components["schemas"]["GroupMapping"];
export type IdentityProviderInput =
  components["schemas"]["IdentityProviderInput"];

export const identityKeys = {
  all: ["identity-providers"] as const,
};

/** How people sign in, and what signing in grants them. */
export function useIdentityProviders() {
  return useQuery({
    queryKey: identityKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/identity-providers")),
  });
}

export function usePutIdentityProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; body: IdentityProviderInput }) =>
      unwrap(
        await api.PUT("/admin/identity-providers/{id}", {
          params: { path: { id: input.id } },
          body: input.body,
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: identityKeys.all }),
  });
}

export function useDeleteIdentityProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.DELETE("/admin/identity-providers/{id}", {
          params: { path: { id } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: identityKeys.all }),
  });
}
