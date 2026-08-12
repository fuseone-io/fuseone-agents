import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type RegisteredScope = components["schemas"]["RegisteredScope"];

export const scopeKeys = {
  all: ["scopes"] as const,
  list: () => [...scopeKeys.all, "list"] as const,
};

/**
 * The areas the caller reaches.
 *
 * Read by every role, because this is what the context switcher offers and
 * somebody who cannot see the areas they work in cannot choose one. Held for a
 * while: an area is registered by hand, so it does not change under a reader.
 */
export function useScopes() {
  return useQuery({
    queryKey: scopeKeys.list(),
    queryFn: async () => unwrap(await api.GET("/admin/scopes")),
    staleTime: 5 * 60_000,
  });
}

/**
 * Registers an area, or relabels the one the name folds onto.
 *
 * The reply carries the canonical id — "Risco de Crédito" becomes
 * `risco-de-credito` — so a caller never has to guess what it stored.
 */
export function useRegisterScope() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      company: string;
      name: string;
      label?: string;
    }) => unwrap(await api.POST("/admin/scopes", { body: input })),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: scopeKeys.all }),
  });
}

export function useDeleteScope() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scope: string) =>
      unwrap(
        await api.DELETE("/admin/scopes/{scope}", {
          params: { path: { scope } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: scopeKeys.all }),
  });
}
