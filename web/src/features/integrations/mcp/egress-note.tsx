import { Network } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { MCPServer } from "@/features/integrations/api";

export function EgressNote({
  egress,
}: {
  egress?: MCPServer["egress"] | null;
}) {
  const { t } = useTranslation();
  if (!egress) return null;

  return (
    <div className="flex gap-2 rounded-lg border border-warning/30 bg-warning-surface/50 p-3">
      <Network className="mt-0.5 size-4 shrink-0 text-warning" />
      <div className="space-y-1">
        <p className="text-xs font-medium">{t("mcp.egressTitle")}</p>
        <p className="text-xs text-muted-foreground">{messageOf(egress, t)}</p>
      </div>
    </div>
  );
}

function messageOf(
  egress: NonNullable<MCPServer["egress"]>,
  t: ReturnType<typeof useTranslation>["t"],
) {
  switch (egress.policy) {
    case "metadata_refused":
      return t("mcp.egressMetadataRefused");
    case "proxy_requested":
      return t("mcp.egressProxyRequested");
    case "proxy_with_network_policy":
      return t("mcp.egressProxyWithNetworkPolicy");
    case "unconstrained_local_process":
      return t("mcp.egressLocalUnconstrained");
    default:
      return t("mcp.egressUnknown");
  }
}
