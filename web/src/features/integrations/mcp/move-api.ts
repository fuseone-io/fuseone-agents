import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { ifMatch } from "@/lib/api/revision";
import { integrationKeys } from "@/features/integrations/api";

/**
 * Moves a server out of the installation (NT-019), against the revision the
 * page read. Refused or not, what is on screen may be older than what is
 * stored, so the integrations are read again either way.
 */
export function useMoveMCPServer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      name: string;
      etag: string;
      company: string;
      area: string;
      strand: boolean;
    }) =>
      unwrap(
        await api.POST("/admin/integrations/mcp-servers/{name}/move", {
          params: { path: { name: input.name }, header: ifMatch(input.etag) },
          body: {
            company: input.company,
            ...(input.area ? { area: input.area } : {}),
            ...(input.strand ? { strand: true } : {}),
          },
        }),
      ),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}
