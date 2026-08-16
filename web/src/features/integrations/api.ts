import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type MCPServer = components["schemas"]["MCPServer"];
export type ModelProvider = components["schemas"]["ModelProvider"];
export type IntegrationHealth = components["schemas"]["IntegrationHealth"];

export const integrationKeys = {
  all: ["integrations"] as const,
  list: () => [...integrationKeys.all, "list"] as const,
};

export function useIntegrations() {
  return useQuery({
    queryKey: integrationKeys.list(),
    queryFn: async () => unwrap(await api.GET("/admin/integrations")),
  });
}

export function usePutMCPServer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      name: string;
      transport: "stdio" | "http";
      command: string;
      args: string[];
      url: string;
      token: string;
      acceptsLocalExecution: boolean;
      enabled: boolean;
    }) =>
      unwrap(
        await api.PUT("/admin/integrations/mcp-servers/{name}", {
          params: { path: { name: input.name } },
          body: {
            transport: input.transport,
            command: input.command,
            args: input.args,
            url: input.url,
            // Omitted rather than emptied: an empty one would read as
            // "clear it", and correcting a URL must not drop the token.
            token: input.token || undefined,
            acceptsLocalExecution: input.acceptsLocalExecution,
            enabled: input.enabled,
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}

export function useDeleteMCPServer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(
        await api.DELETE("/admin/integrations/mcp-servers/{name}", {
          params: { path: { name } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}

/**
 * An omitted key keeps the stored one, so the form sends nothing rather than
 * an empty string when the operator left the field alone.
 */
export function usePutProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      name: string;
      kind: "anthropic" | "openai_compatible";
      baseUrl: string;
      apiKey?: string;
      enabled: boolean;
    }) =>
      unwrap(
        await api.PUT("/admin/integrations/providers/{name}", {
          params: { path: { name: input.name } },
          body: {
            kind: input.kind,
            baseUrl: input.baseUrl,
            enabled: input.enabled,
            ...(input.apiKey ? { apiKey: input.apiKey } : {}),
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}

export function useDeleteProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(
        await api.DELETE("/admin/integrations/providers/{name}", {
          params: { path: { name } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}
