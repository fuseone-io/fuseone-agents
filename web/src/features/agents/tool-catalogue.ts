import type { Tool } from "@/lib/api/client";

/** One server, and the tools it offers. */
export interface ToolGroup {
  server: string;
  tools: Tool[];
  /** How many of this server's tools this agent may invoke. */
  granted: number;
}

/**
 * The catalogue, grouped by the server each tool came from.
 *
 * A flat list works at eight tools and stops working at eighty: a tool is
 * named after its server, so the list is already sorted into groups and
 * refusing to draw them makes the reader do it. The count per server is what
 * answers "what can this agent reach in the ERP" without opening anything.
 */
export function grouped(catalogue: Tool[], granted: string[]): ToolGroup[] {
  const byServer = new Map<string, Tool[]>();
  for (const tool of catalogue) {
    byServer.set(tool.server, [...(byServer.get(tool.server) ?? []), tool]);
  }

  return [...byServer.entries()]
    .map(([server, tools]) => ({
      server,
      tools: [...tools].sort((a, b) => a.toolId.localeCompare(b.toolId)),
      granted: tools.filter((t) => granted.includes(t.toolId)).length,
    }))
    .sort((a, b) => a.server.localeCompare(b.server));
}

/**
 * Narrows the catalogue to what somebody typed.
 *
 * Matches the identifier and the description, because an author looking for
 * "refund" knows what they want to do and not what somebody called it.
 */
export function matching(catalogue: Tool[], query: string): Tool[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return catalogue;
  return catalogue.filter(
    (tool) =>
      tool.toolId.toLowerCase().includes(needle) ||
      (tool.description ?? "").toLowerCase().includes(needle),
  );
}
