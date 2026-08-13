import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

export const retentionKeys = {
  all: ["retention"] as const,
};

/** How long this installation keeps content. */
export function useRetention() {
  return useQuery({
    queryKey: retentionKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/retention")),
  });
}

export function useSetRetention() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (days: number) =>
      unwrap(await api.PUT("/admin/retention", { body: { days } })),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: retentionKeys.all }),
  });
}

/** Erases what a set of runs was about, on somebody's request. */
export function useEraseContent() {
  return useMutation({
    mutationFn: async (input: { runs: string[]; reason: string }) =>
      unwrap(await api.POST("/admin/erasures", { body: input })),
  });
}
