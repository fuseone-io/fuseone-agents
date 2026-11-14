import type { ServerRecipe } from "@/features/integrations/mcp/api";
import type { MCPServer } from "@/features/integrations/api";

/**
 * Whether the caller may configure a catalogue entry (NT-019).
 *
 * A recipe nobody connected needs only the right to create somewhere. A
 * connected server needs more: the caller may change it (`editable`), and it
 * is one the console manages at all (`managed`), since one passed to the
 * process by flag or environment has no row to change. Absent means an older
 * backend, which answered for the platform alone.
 */
export function configurable(canCreate: boolean, server: MCPServer | null | undefined): boolean {
  if (!canCreate) return false;
  return !server || (server.editable !== false && server.managed !== false);
}

/**
 * Whether a recipe is reached only by a local command (NT-019). Outside the
 * installation a server is remote: a local process runs on the platform's
 * worker, which no company or area commands, and the server refuses one
 * stored there. So a caller who places servers only in their own scopes
 * (`anyScope` false) reads such a recipe and is not offered to connect it.
 */
export function localOnly(
  recipe: Pick<ServerRecipe, "transport" | "url"> | null | undefined,
  anyScope: boolean,
): boolean {
  if (anyScope || !recipe) return false;
  return recipe.transport !== "http" && !recipe.url;
}
