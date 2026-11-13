import type { MCPServer } from "@/features/integrations/api";

/**
 * The target a server's tools are asked for (NT-019): the scope it is stored
 * at, `company` or `company/area`. Nothing for the installation's own, which
 * every reader is answered without a target.
 */
export function serverTarget(
  server: Pick<MCPServer, "scopeKind" | "company" | "area"> | undefined,
): string | undefined {
  if (!server?.company) return undefined;
  if (server.scopeKind === "company") return server.company;
  if (server.scopeKind === "area" && server.area) {
    return `${server.company}/${server.area}`;
  }
  return undefined;
}
