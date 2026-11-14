import { useQueries } from "@tanstack/react-query";
import { toolsQuery, type Tool } from "@/features/admin/api";
import type { MCPServer } from "@/features/integrations/api";
import type { MeScope } from "@/features/session/api";

/**
 * The tools of the servers the catalogue shows (NT-019).
 *
 * Without a target the tool catalogue answers the platform's reader every
 * tool, and anyone else the installation's alone. A target is answered only
 * inside the caller's own scopes, and holds what that scope inherits. So for
 * anyone else it is asked for the installation, for each scope they configure
 * (whose answer carries a server inherited from their company), and for the
 * scope of each server stored inside one of those (an area of their company).
 * Never for a scope above theirs, which the server would not find.
 */
export function useCatalogueTools(servers: MCPServer[], platform: boolean, own: MeScope[]) {
  const targets = platform ? [undefined] : [undefined, ...catalogueTargets(servers, own)];
  const answers = useQueries({ queries: targets.map((target) => toolsQuery(target)) });

  const seen = new Set<string>();
  const items: Tool[] = [];
  for (const answer of answers) {
    for (const tool of answer.data?.items ?? []) {
      if (seen.has(tool.toolId)) continue;
      seen.add(tool.toolId);
      items.push(tool);
    }
  }
  return {
    data: { items },
    isLoading: answers.some((answer) => answer.isLoading),
    error: answers.find((answer) => answer.error)?.error ?? null,
    refetch: () => answers.forEach((answer) => void answer.refetch()),
  };
}

/** The targets the caller may ask for: their own scopes, and servers' scopes inside them. */
export function catalogueTargets(servers: MCPServer[], own: MeScope[]): string[] {
  const held = own.filter((scope) => scope.company !== "");
  const targets = new Set(held.map(pathOf));
  for (const server of servers) {
    if (server.scopeKind !== "company" && server.scopeKind !== "area") continue;
    const stored = { company: server.company ?? "", area: server.scopeKind === "area" ? (server.area ?? "") : "" };
    if (stored.company === "") continue;
    if (held.some((scope) => contains(scope, stored))) targets.add(pathOf(stored));
  }
  return [...targets].sort();
}

function contains(outer: MeScope, inner: MeScope) {
  return outer.company === inner.company && (outer.area === "" || outer.area === inner.area);
}

function pathOf(scope: MeScope) {
  return scope.area ? `${scope.company}/${scope.area}` : scope.company;
}
