import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type AuthoringChoice = components["schemas"]["AuthoringChoice"];

export const authoringKeys = { all: ["authoring"] as const };

/** Which connected provider writes the drafts. */
export function useAuthoring() {
  return useQuery({
    queryKey: authoringKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/authoring")),
  });
}

export function useSetAuthoring() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (choice: AuthoringChoice) =>
      unwrap(await api.PUT("/admin/authoring", { body: choice })),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: authoringKeys.all }),
  });
}
