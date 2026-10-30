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

export type BrandAsset = components["schemas"]["BrandAsset"];

/**
 * Sends the file's bytes as they are. The server decides what the file is
 * from its content; the name and the browser's guess at its type are not
 * sent, because nothing should rely on them.
 */
export function useUploadBrandAsset() {
  return useMutation({
    mutationFn: async (file: File) =>
      unwrap(
        await api.POST("/admin/branding/assets", {
          // The contract types a binary body as string; the request carries
          // the File itself, untouched by any serializer.
          body: file as unknown as string,
          bodySerializer: (body) => body,
          headers: { "Content-Type": "application/octet-stream" },
        }),
      ),
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
