import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

export const companyKeys = { all: ["companies"] as const };

/**
 * Every company, withdrawn ones included.
 *
 * This is the administrative listing and needs authority over the
 * installation. The list somebody chooses a working context from is a
 * different one — it comes from their own grants and shows only what they
 * reach.
 */
export function useCompanies() {
  return useQuery({
    queryKey: companyKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/companies")),
    retry: false,
  });
}

export function useCreateCompany() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (body: { id: string; label?: string }) =>
      unwrap(await api.POST("/admin/companies", { body })),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: companyKeys.all }),
  });
}

export function useUpdateCompany() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      company,
      ...body
    }: {
      company: string;
      label?: string;
      archived?: boolean;
    }) =>
      unwrap(
        await api.PATCH("/admin/companies/{company}", {
          params: { path: { company } },
          body,
        }),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: companyKeys.all }),
  });
}
