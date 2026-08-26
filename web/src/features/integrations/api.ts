import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type MCPServer = components["schemas"]["MCPServer"];
export type MCPUserCredential = components["schemas"]["MCPUserCredential"];
export type MCPOAuthGrant = components["schemas"]["MCPOAuthGrant"];
export type ModelProvider = components["schemas"]["ModelProvider"];
export type IntegrationHealth = components["schemas"]["IntegrationHealth"];
export type GovernedConnector = components["schemas"]["GovernedConnector"];
export type ConnectorInstance = components["schemas"]["ConnectorInstance"];
export type ConnectorInstanceInput =
  components["schemas"]["ConnectorInstanceInput"];
export type ConnectorScopeKind = components["schemas"]["ConnectorScopeKind"];
export type ConnectorVaultConfig = components["schemas"]["ConnectorVaultConfig"];

export const integrationKeys = {
  all: ["integrations"] as const,
  list: () => [...integrationKeys.all, "list"] as const,
  mcpCredentials: () => [...integrationKeys.all, "mcp-credentials"] as const,
  connectorCatalog: () =>
    [...integrationKeys.all, "connectors", "catalog"] as const,
  connectorInstances: () =>
    [...integrationKeys.all, "connectors", "instances"] as const,
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
      protocolMode: "auto" | "legacy";
      command: string;
      args: string[];
      url: string;
      /**
       * Absent leaves the stored one; an empty string removes it.
       *
       * Two different requests that a truthiness check collapses into one —
       * which is how the page grew a revoke button that did not revoke: it
       * sent an empty token, the client dropped it as falsy, and the server
       * read the silence as "keep what you have".
       */
      token?: string;
      /**
       * Exact HTTP headers for remote credentials that are not bearer tokens.
       * Absent leaves stored headers alone; an empty object revokes them.
       */
      headers?: Record<string, string>;
      /**
       * Absent leaves the stored OAuth grant; an empty object removes it.
       * A non-empty grant becomes the active HTTP credential and replaces a
       * stored bearer.
       */
      oauth?: MCPOAuthGrant;
      /**
       * Variables a local server is given. Omitted when the field was left
       * alone, because an empty object means "clear them" and an absent one
       * means "leave what is stored" — an edit to a command must not silently
       * drop a credential.
       */
      env?: Record<string, string>;
      /**
       * Managed configuration content for a local server. Omitted keeps the
       * stored file; an empty string removes it.
       */
      configFile?: string;
      /**
       * Variable that receives the managed config file path. Empty means the
       * platform default; omitted leaves the stored choice alone.
       */
      configFileEnv?: string;
      rateLimit?: components["schemas"]["MCPRateLimit"];
      cache?: components["schemas"]["MCPResultCache"];
      stdioEgress?: components["schemas"]["MCPStdioEgress"];
      acceptsLocalExecution: boolean;
      enabled: boolean;
    }) =>
      unwrap(
        await api.PUT("/admin/integrations/mcp-servers/{name}", {
          params: { path: { name: input.name } },
          body: {
            transport: input.transport,
            protocolMode: input.protocolMode,
            command: input.command,
            args: input.args,
            url: input.url,
            // Omitted rather than emptied: an empty one would read as
            // "clear it", and correcting a URL must not drop the token.
            token: input.token,
            headers: input.headers,
            oauth: input.oauth,
            env: input.env,
            configFile: input.configFile,
            configFileEnv: input.configFileEnv,
            rateLimit: input.rateLimit,
            cache: input.cache,
            stdioEgress: input.stdioEgress,
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

export function useMCPUserCredentials(enabled = true) {
  return useQuery({
    queryKey: integrationKeys.mcpCredentials(),
    queryFn: async () => unwrap(await api.GET("/integrations/mcp-credentials")),
    enabled,
  });
}

export function useConnectorCatalog(enabled = true) {
  return useQuery({
    queryKey: integrationKeys.connectorCatalog(),
    queryFn: async () =>
      unwrap(await api.GET("/admin/integrations/connectors/catalog")),
    enabled,
  });
}

export function useConnectorInstances(enabled = true) {
  return useQuery({
    queryKey: integrationKeys.connectorInstances(),
    queryFn: async () =>
      unwrap(await api.GET("/admin/integrations/connectors/instances")),
    enabled,
  });
}

export function usePutConnectorInstance() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      name: string;
      body: ConnectorInstanceInput;
    }) =>
      unwrap(
        await api.PUT("/admin/integrations/connectors/instances/{name}", {
          params: { path: { name: input.name } },
          body: input.body,
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}

export function useDeleteConnectorInstance() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (instance: ConnectorInstance) =>
      unwrap(
        await api.DELETE("/admin/integrations/connectors/instances/{name}", {
          params: {
            path: { name: instance.name },
            query: {
              scopeKind: instance.scopeKind,
              company: instance.company,
              area: instance.area,
            },
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}

export function usePutMCPUserCredential() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      name: string;
      token?: string;
      headers?: Record<string, string>;
      oauth?: MCPOAuthGrant;
    }) =>
      unwrap(
        await api.PUT("/integrations/mcp-credentials/{name}", {
          params: { path: { name: input.name } },
          body: {
            token: input.token,
            headers: input.headers,
            oauth: input.oauth,
          },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: integrationKeys.mcpCredentials(),
      }),
  });
}

export function useDeleteMCPUserCredential() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(
        await api.DELETE("/integrations/mcp-credentials/{name}", {
          params: { path: { name } },
        }),
      ),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: integrationKeys.mcpCredentials(),
      }),
  });
}

export function useProbeMCPServer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(
        await api.POST("/admin/integrations/mcp-servers/{name}/probe", {
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
