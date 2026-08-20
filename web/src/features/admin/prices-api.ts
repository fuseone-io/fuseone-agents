import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type ModelPrice = components["schemas"]["ModelPrice"];

export const priceKeys = { all: ["prices"] as const };

/** What this installation pays per model.
 *
 * Configured rates are the installation's contract override. Market defaults
 * are bundled with the release as reference values; they do not feed
 * Cost.Micros until an operator records a local rate.
 */
export function usePrices() {
  return useQuery({
    queryKey: priceKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/prices")),
  });
}

export function usePutPrice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (price: ModelPrice) =>
      unwrap(await api.PUT("/admin/prices", { body: price })),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: priceKeys.all }),
  });
}

export function useDeletePrice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (at: { provider: string; model: string }) =>
      unwrap(
        await api.DELETE("/admin/prices/{provider}/{model}", {
          params: { path: at },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: priceKeys.all }),
  });
}
