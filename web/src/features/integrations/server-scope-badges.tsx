import type { MCPServer } from "@/features/integrations/api";
import { ScopeBadges } from "@/features/integrations/scope/scope-badges";

/** Where a server is stored, and whether the caller only inherits it (NT-019). */
export function ServerScopeBadges({ server }: { server: MCPServer }) {
  return <ScopeBadges row={server} />;
}
