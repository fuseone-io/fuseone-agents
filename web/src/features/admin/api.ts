import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type Tool = components["schemas"]["Tool"];
export type Effect = components["schemas"]["Effect"];
export type MCPServer = components["schemas"]["MCPServer"];
export type ModelProvider = components["schemas"]["ModelProvider"];
export type AdminEvent = components["schemas"]["AdminEvent"];

export const adminKeys = {
  all: ["admin"] as const,
  tools: () => [...adminKeys.all, "tools"] as const,
  integrations: () => [...adminKeys.all, "integrations"] as const,
  events: (target?: string) => [...adminKeys.all, "events", target ?? ""] as const,
};

export function useTools() {
  return useQuery({
    queryKey: adminKeys.tools(),
    queryFn: async () => unwrap(await api.GET("/admin/tools")),
  });
}

export function useIntegrations() {
  return useQuery({
    queryKey: adminKeys.integrations(),
    queryFn: async () => unwrap(await api.GET("/admin/integrations")),
  });
}

export function useAdminEvents(target?: string) {
  return useQuery({
    queryKey: adminKeys.events(target),
    queryFn: async () =>
      unwrap(await api.GET("/admin/events", { params: { query: { target, limit: 50 } } })),
  });
}

/**
 * Classifying is the single point where write access enters the platform, so
 * the trail is invalidated alongside the catalogue: the change and the record
 * of it are shown together or the screen is lying about one of them.
 */
export function useClassifyTool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { toolId: string; effect: Effect; untrusted: boolean; reason?: string }) =>
      unwrap(
        await api.PUT("/admin/tools/{toolId}/classification", {
          params: { path: { toolId: input.toolId } },
          body: { effect: input.effect, untrusted: input.untrusted, reason: input.reason },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: adminKeys.all });
    },
  });
}

export function usePutMCPServer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { name: string; command: string; args: string[]; enabled: boolean }) =>
      unwrap(
        await api.PUT("/admin/integrations/mcp-servers/{name}", {
          params: { path: { name: input.name } },
          body: { command: input.command, args: input.args, enabled: input.enabled },
        }),
      ),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: adminKeys.all }),
  });
}

export function useDeleteMCPServer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(await api.DELETE("/admin/integrations/mcp-servers/{name}", { params: { path: { name } } })),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: adminKeys.all }),
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
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: adminKeys.all }),
  });
}

export function useDeleteProvider() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(await api.DELETE("/admin/integrations/providers/{name}", { params: { path: { name } } })),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: adminKeys.all }),
  });
}
