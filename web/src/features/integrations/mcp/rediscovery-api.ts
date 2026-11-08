import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";
import { integrationKeys } from "@/features/integrations/api";

export type MCPRediscovery = components["schemas"]["MCPRediscovery"];
export type MCPRediscoveryReceipt = components["schemas"]["MCPRediscoveryReceipt"];

// While the latest request may still be answered, the answers are read again:
// a worker answers on its next pass, and the screen should notice without a
// reload. That is while a recorded worker is behind, and while none has
// recorded anything yet, since the first answer is then still to come. Once
// every recorded worker has answered there is nothing left to wait for.
const waitingEvery = 5_000;

export function behind(status: MCPRediscovery | undefined): boolean {
  if (!status || status.generation === 0) return false;
  if (status.receipts.length === 0) return true;
  return status.receipts.some((r) => r.generation < status.generation);
}

export function useMCPRediscovery(name: string) {
  return useQuery({
    queryKey: integrationKeys.mcpRediscovery(name),
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/integrations/mcp-servers/{name}/rediscovery", {
          params: { path: { name } },
        }),
      ),
    refetchInterval: (query) => (behind(query.state.data) ? waitingEvery : false),
  });
}

export function useRequestMCPRediscovery(name: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/admin/integrations/mcp-servers/{name}/rediscovery", {
          params: { path: { name } },
        }),
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: integrationKeys.mcpRediscovery(name) }),
  });
}
