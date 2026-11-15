import type { MCPServer } from "@/features/integrations/api";
import type { IntegrationAccess } from "@/features/integrations/integration-access";

/**
 * Whether the page offers to move the server (NT-019): the platform's
 * administration, for a remote server still stored at the installation. A
 * local server stays the installation's, and one already at a company or area
 * is not moved again.
 */
export function moveOffered(
  access: Pick<IntegrationAccess, "platformConfigure">,
  server: Pick<MCPServer, "scopeKind" | "transport">,
): boolean {
  return (
    access.platformConfigure &&
    (server.scopeKind ?? "installation") === "installation" &&
    server.transport === "http"
  );
}
