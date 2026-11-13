import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { MCPServer } from "@/features/integrations/api";
import { inheritedFrom, scopeLabel } from "@/features/integrations/scope/scope-label";

/**
 * Where a server is stored, and whether the caller only inherits it (NT-019).
 * A server that predates scopes says nothing, rather than a guess.
 */
export function ServerScopeBadges({ server }: { server: MCPServer }) {
  const { t } = useTranslation();
  if (!server.scopeKind) return null;
  return (
    <>
      <Badge variant="secondary" className="font-mono">
        {scopeLabel(server, t)}
      </Badge>
      {server.editable === false && (
        <Badge variant="outline">{t(inheritedFrom(server))}</Badge>
      )}
    </>
  );
}
