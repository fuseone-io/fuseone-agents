import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type Branding = components["schemas"]["Branding"];

export const brandingKeys = {
  all: ["branding"] as const,
  public: () => [...brandingKeys.all, "public"] as const,
  admin: () => [...brandingKeys.all, "admin"] as const,
};

export function useBranding() {
  return useQuery({
    queryKey: brandingKeys.public(),
    queryFn: async () => unwrap(await api.GET("/branding")),
    staleTime: 60_000,
    retry: 1,
  });
}

export function useAdminBranding() {
  return useQuery({
    queryKey: brandingKeys.admin(),
    queryFn: async () => unwrap(await api.GET("/admin/branding")),
  });
}

export function useSetAdminBranding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (branding: Branding) =>
      unwrap(await api.PUT("/admin/branding", { body: branding })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: brandingKeys.all });
    },
  });
}
