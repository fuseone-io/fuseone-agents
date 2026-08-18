import type { ServerRecipe } from "@/features/integrations/mcp/api";

export type AuthMode = NonNullable<ServerRecipe["authModes"]>[number];

export type RemoteAuthPlan = {
  known: boolean;
  modes: AuthMode[];
  bearer: AuthMode | null;
  oauth: AuthMode | null;
  noAuth: AuthMode | null;
  unsupported: AuthMode[];
};

/**
 * What the runtime can actually store for a remote MCP connection.
 *
 * The recipe says what the server expects; this says which of those shapes the
 * console can truthfully edit today. Unknown custom servers keep the historical
 * bearer-or-OAuth affordance, because there is no recipe to narrow it.
 */
export function remoteAuthPlan(
  modes: AuthMode[] | null | undefined,
  known: boolean,
): RemoteAuthPlan {
  if (!known) {
    return {
      known: false,
      modes: [],
      bearer: { type: "bearer", principal: "service" },
      oauth: { type: "oauth2", principal: "user" },
      noAuth: null,
      unsupported: [],
    };
  }

  const all = modes ?? [];
  const bearer = all.find(isPlainBearer) ?? null;
  const oauth = all.find((mode) => mode.type === "oauth2") ?? null;
  const noAuth = all.find((mode) => mode.type === "none") ?? null;
  return {
    known: true,
    modes: all,
    bearer,
    oauth,
    noAuth,
    unsupported: all.filter(
      (mode) => mode.type !== "none" && mode.type !== "oauth2" && !isPlainBearer(mode),
    ),
  };
}

function isPlainBearer(mode: AuthMode) {
  if (mode.type !== "bearer") return false;
  const header = mode.header?.trim().toLowerCase();
  const prefix = mode.prefix?.trim().toLowerCase();
  return (
    (header === undefined || header === "" || header === "authorization") &&
    (prefix === undefined || prefix === "" || prefix === "bearer")
  );
}
