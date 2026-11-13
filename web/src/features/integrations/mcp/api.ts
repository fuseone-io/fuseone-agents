import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { ifMatch, written } from "@/lib/api/revision";
import type { components } from "@/lib/api/schema.gen";
import {
  integrationKeys,
  storedScope,
  type MCPServer,
} from "@/features/integrations/api";

export type ServerRecipe = components["schemas"]["ServerRecipe"];

export const recipeKeys = { all: ["recipes"] as const };

/**
 * What this platform has read about servers other people publish.
 *
 * Recipes, not connectors. Nothing here is hosted, supported or endorsed —
 * a recipe fills the form and decides nothing, which is why the card carries
 * the publisher and whose page it was read from rather than a badge.
 */
export function useRecipes(enabled = true) {
  return useQuery({
    queryKey: recipeKeys.all,
    queryFn: async () =>
      unwrap(await api.GET("/admin/integrations/recipes", {})),
    enabled,
  });
}

/**
 * Which of a server's tools this installation brought in.
 *
 * Sent as the whole list rather than as a change, because that is what the
 * field is: the answer to "of what you offer, these". The rest of the server
 * travels with it — a PUT replaces the record, so sending the surface alone
 * would blank the address it is reached at.
 */
export function useSetSurface() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      name: string;
      /** Where it is stored, as read: a server's scope does not move (NT-019). */
      scope: Pick<MCPServer, "scopeKind" | "company" | "area">;
      surface: string[];
      transport: "stdio" | "http";
      protocolMode: "auto" | "legacy";
      command: string;
      args: string[];
      url: string;
      enabled: boolean;
      acceptsLocalExecution: boolean;
      /** The revision the server's page read. */
      etag: string;
    }) =>
      written(
        await api.PUT("/admin/integrations/mcp-servers/{name}", {
          params: { path: { name: input.name }, header: ifMatch(input.etag) },
          body: {
            ...storedScope(input.scope),
            transport: input.transport,
            protocolMode: input.protocolMode,
            command: input.command,
            args: input.args,
            url: input.url,
            surface: input.surface,
            enabled: input.enabled,
            acceptsLocalExecution: input.acceptsLocalExecution,
          },
        }),
      ),
    // Returned, so the write is not done until the server has been read
    // again: the next write on the page answers the revision this one made.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: integrationKeys.all }),
  });
}
