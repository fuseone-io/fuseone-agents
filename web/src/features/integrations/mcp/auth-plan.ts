import type { ServerRecipe } from "@/features/integrations/mcp/api";

export type AuthMode = NonNullable<ServerRecipe["authModes"]>[number];

export type RemoteAuthPlan = {
  known: boolean;
  modes: AuthMode[];
  secret: AuthMode | null;
  bearer: AuthMode | null;
  header: AuthMode | null;
  multiHeaders: AuthMode | null;
  dsn: AuthMode | null;
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
      secret: { type: "bearer", principal: "service" },
      bearer: { type: "bearer", principal: "service" },
      header: null,
      multiHeaders: null,
      dsn: null,
      oauth: { type: "oauth2", principal: "user" },
      noAuth: null,
      unsupported: [],
    };
  }

  const all = modes ?? [];
  const secret = all.find(isEditableSecret) ?? null;
  const bearer = all.find(isPlainBearer) ?? null;
  const header = all.find(isSingleHeaderCredential) ?? null;
  const multiHeaders = all.find(isMultiHeaderCredential) ?? null;
  const dsn = all.find((mode) => mode.type === "dsn") ?? null;
  const oauth = all.find((mode) => mode.type === "oauth2") ?? null;
  const noAuth = all.find((mode) => mode.type === "none") ?? null;
  return {
    known: true,
    modes: all,
    secret,
    bearer,
    header,
    multiHeaders,
    dsn,
    oauth,
    noAuth,
    unsupported: all.filter(
      (mode) =>
        mode.type !== "none" &&
        mode.type !== "oauth2" &&
        !isPlainBearer(mode) &&
        !isSingleHeaderCredential(mode) &&
        !isMultiHeaderCredential(mode),
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

function isSingleHeaderCredential(mode: AuthMode) {
  return (mode.type === "headers" || mode.type === "basic") && Boolean(mode.header?.trim());
}

function isMultiHeaderCredential(mode: AuthMode) {
  return mode.type === "headers" && (mode.headers ?? []).some((header) => header.trim() !== "");
}

function isEditableSecret(mode: AuthMode) {
  return isPlainBearer(mode) || isSingleHeaderCredential(mode);
}

export function headerNames(mode: AuthMode | null) {
  if (!mode) return [];
  return Array.from(
    new Set((mode.headers ?? []).map((header) => header.trim()).filter(Boolean)),
  );
}

export function dsnEnvMode(modes: AuthMode[] | null | undefined) {
  return modes?.find((mode) => mode.type === "dsn" && mode.env?.trim()) ?? null;
}

export function headerCredential(mode: AuthMode, secret: string): Record<string, string> {
  const header = mode.header?.trim();
  if (!header) return {};
  return { [header]: headerValue(mode, secret) };
}

export function multiHeaderCredential(
  headers: string[],
  values: Record<string, string>,
): Record<string, string> {
  return Object.fromEntries(headers.map((header) => [header, values[header] ?? ""]));
}

function headerValue(mode: AuthMode, secret: string) {
  const prefix = mode.prefix?.trim();
  if (!prefix) return secret;
  return `${prefix}${prefix.endsWith("=") ? "" : " "}${secret}`;
}
