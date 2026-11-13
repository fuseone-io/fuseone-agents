import type { MCPServer } from "@/features/integrations/api";
import type { Me } from "@/features/session/api";

/**
 * Which ruling the console offers on one server's tools (NT-019).
 *
 * - `platform`: the platform's ceiling only, as before.
 * - `owner`: the owner's ruling only, stored at the server's scope.
 * - `choose`: the platform's curator on a company's or an area's server, who
 *   may record either and starts on the ceiling.
 */
export type RulingOffer = "platform" | "owner" | "choose";

/**
 * Decided per server, so nobody is offered a ruling the server would refuse.
 *
 * The platform's curator is read from `platform` (or `can`, from a server that
 * predates it). An owner's ruling needs `tool:classify` at the server's own
 * scope, which `/me` does not say per scope: the console reads it from the
 * server's `editable`, provider:write held where it is stored, because today
 * the roles that hold one in a scope are exactly the roles that hold the
 * other (admin and curator). The server checks `tool:classify` on every
 * ruling either way; this only decides what is offered.
 *
 * Until the session answers, nothing is offered. With no identity configured
 * at all (`null`), every door is open, as in the rest of the console.
 */
export function classifyOffer(
  me: Me | null | undefined,
  server: Pick<MCPServer, "scopeKind" | "editable"> | undefined,
): RulingOffer | null {
  if (me === undefined || !server) return null;
  const scoped = server.scopeKind === "company" || server.scopeKind === "area";
  const platform =
    me === null ||
    (me.platform ?? me.can ?? []).includes("tool:classify");
  if (platform) return scoped ? "choose" : "platform";
  return scoped && server.editable === true ? "owner" : null;
}
