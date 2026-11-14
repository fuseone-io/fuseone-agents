import { useQueries } from "@tanstack/react-query";
import { toolsQuery, type Tool } from "@/features/admin/api";
import type { MCPServer } from "@/features/integrations/api";

/**
 * The tools of the servers the catalogue shows (NT-019).
 *
 * Without a target the tool catalogue answers the platform's reader every
 * tool, and anyone else the installation's alone. So for anyone else it is
 * asked once for the installation and once for the scope of each company or
 * area server they see, and the answers are joined.
 */
export function useCatalogueTools(servers: MCPServer[], platform: boolean) {
  const targets = platform ? [undefined] : [undefined, ...serverTargets(servers)];
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

function serverTargets(servers: MCPServer[]): string[] {
  const targets = new Set<string>();
  for (const server of servers) {
    if (server.scopeKind !== "company" && server.scopeKind !== "area") continue;
    if (!server.company) continue;
    targets.add(server.scopeKind === "area" && server.area ? `${server.company}/${server.area}` : server.company);
  }
  return [...targets].sort();
}
