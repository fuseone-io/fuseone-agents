import { useMemo, useState } from "react";
import { useIntegrations, type MCPServer } from "@/features/integrations/api";
import { useTools, type Tool } from "@/features/admin/api";
import { remoteNameOf } from "@/features/integrations/mcp/tool-names";
import { serverTarget } from "@/features/integrations/mcp/server-target";

/**
 * One server, its tools, and the surface as it is being edited.
 *
 * The chosen set is local until it is saved, because the whole point of the
 * screen is to see what a change would cost before making it — a checkbox that
 * wrote straight through would show the impact of a decision already taken.
 *
 * A server whose surface nobody has ever chosen starts with everything ticked,
 * which is what the runtime does with it. The alternative would show an empty
 * surface for a server that is offering all of them.
 */
export function useMCPServer(
  name: string | undefined,
  /**
   * The server as read by name. Its surface is the one shown and edited,
   * because its revision is the one a save answers: the listing may still
   * hold a surface somebody has since replaced.
   */
  stored?: MCPServer,
) {
  const integrations = useIntegrations();
  const [edited, setEdited] = useState<Set<string> | null>(null);

  const server = useMemo(
    () => integrations.data?.mcpServers?.find((s) => s.name === name),
    [integrations.data, name],
  );
  // Asked at the server's own scope once it is known: without a target, an
  // area's administrator is answered the installation's tools alone, and
  // their own server would show none of its own.
  const tools = useTools(integrations.isSuccess, serverTarget(server));

  const its = useMemo(
    () => (tools.data?.items ?? []).filter((t: Tool) => t.server === name),
    [tools.data, name],
  );

  const surface = useMemo(() => {
    if (stored?.surface) return new Set(stored.surface);
    return new Set(its.map(remoteNameOf));
  }, [stored, its]);

  const chosen = edited ?? surface;

  return {
    server,
    tools: its,
    /*
      The whole catalogue, for the one question that is not about this server:
      what undoes a tool may live on another. A compensator list narrowed to
      the server being looked at offers an undo the platform can reach and
      hides every other one.
    */
    catalogue: tools.data?.items ?? [],
    chosen,
    dirty: edited !== null,
    toggle(remoteName: string, next: boolean) {
      const updated = new Set(chosen);
      if (next) updated.add(remoteName);
      else updated.delete(remoteName);
      setEdited(updated);
    },
    reset() {
      setEdited(null);
    },
    isLoading: integrations.isLoading || tools.isLoading,
    error: integrations.error ?? tools.error,
    refetch: () => {
      void integrations.refetch();
      void tools.refetch();
    },
  };
}

export type { MCPServer };
