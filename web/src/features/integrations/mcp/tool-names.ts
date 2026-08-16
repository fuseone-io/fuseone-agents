import type { Tool } from "@/features/admin/api";

/**
 * What the server calls a tool, which is what a surface is stored by.
 *
 * The platform namespaces a tool by its server — `crm.lookup` is ours and
 * `lookup` is theirs — and a surface stored in our names would stop matching
 * the moment the same server were registered under a different one.
 */
export function remoteNameOf(tool: Tool): string {
  const prefix = `${tool.server}.`;
  return tool.toolId.startsWith(prefix)
    ? tool.toolId.slice(prefix.length)
    : tool.toolId;
}
