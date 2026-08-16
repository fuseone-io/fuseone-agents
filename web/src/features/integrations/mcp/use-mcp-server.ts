import { useMemo, useState } from "react";
import { useIntegrations, type MCPServer } from "@/features/integrations/api";
import { useTools, type Tool } from "@/features/admin/api";
import { remoteNameOf } from "@/features/integrations/mcp/tool-names";

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
export function useMCPServer(name: string | undefined) {
  const integrations = useIntegrations();
  const tools = useTools();
  const [edited, setEdited] = useState<Set<string> | null>(null);

  const server = useMemo(
    () => integrations.data?.mcpServers?.find((s) => s.name === name),
    [integrations.data, name],
  );

  const its = useMemo(
    () => (tools.data?.items ?? []).filter((t: Tool) => t.server === name),
    [tools.data, name],
  );

  const stored = useMemo(() => {
    if (server?.surface) return new Set(server.surface);
    return new Set(its.map(remoteNameOf));
  }, [server, its]);

  const chosen = edited ?? stored;

  return {
    server,
    tools: its,
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
