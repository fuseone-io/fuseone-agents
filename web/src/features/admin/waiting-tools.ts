import type { Tool } from "@/features/admin/api";

export function waitingFor(tools: Tool[]): Tool[] {
  return tools.filter((tool) => tool.effect === "unknown" || tool.stale === true);
}
