import type { Scope, Tool } from "@/features/admin/api";
import type { Me, MeScope } from "@/features/session/api";

/**
 * Which ruling the console offers on one tool (NT-019).
 *
 * - `platform`: the platform's ceiling only, as before.
 * - `owner`: the owner's ruling only, stored at the tool's server's scope.
 * - `choose`: the platform's curator on a company's or an area's tool, who
 *   may record either and starts on the ceiling.
 */
export type RulingOffer = "platform" | "owner" | "choose";

/**
 * Decided per tool, so nobody is offered a ruling the server would refuse.
 *
 * The platform's curator is read from `platform` (or `can`, from a server that
 * predates it). An owner's ruling needs tool:classify at a scope containing
 * the tool's own, which `/me` lists as `classify`: a company contains its
 * areas, an area only itself. The installation's tools (company `*`) are the
 * platform's alone. The server checks every ruling either way; this only
 * decides what is offered.
 *
 * Until the session answers, nothing is offered. With no identity configured
 * at all (`null`), every door is open, as in the rest of the console.
 */
export function classifyOffer(
  me: Me | null | undefined,
  tool: Pick<Tool, "scope"> | undefined,
): RulingOffer | null {
  if (me === undefined || !tool) return null;
  const owned = tool.scope && tool.scope.company !== "*" ? tool.scope : undefined;
  const platform =
    me === null || (me.platform ?? me.can ?? []).includes("tool:classify");
  if (platform) return owned ? "choose" : "platform";
  if (!owned) return null;
  return (me.classify ?? []).some((held) => contains(held, owned)) ? "owner" : null;
}

function contains(held: MeScope, scope: Scope): boolean {
  if (held.company !== scope.company) return false;
  return held.area === "" || held.area === (scope.area ?? "");
}
