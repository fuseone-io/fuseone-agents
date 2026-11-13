import { useTranslation } from "react-i18next";
import { Server } from "lucide-react";
import { EmptyState } from "@/components/shared/states";
import { LoadMore } from "@/components/shared/load-more";
import { IntegrationsSection } from "@/features/integrations/integrations-section";
import { ServerCard } from "@/features/integrations/server-card";
import type { MCPServer } from "@/features/integrations/api";
import { useVisibleItems } from "@/hooks/use-visible-items";

/**
 * The MCP servers the caller can see: every one, for the platform; its own
 * and those it inherits, for an area's configurer (NT-019).
 */
export function ConnectedServersSection({
  servers,
  onAdd,
  onEdit,
}: {
  servers: MCPServer[];
  /** Absent for a caller who may not create a server. */
  onAdd?: () => void;
  onEdit: (server: MCPServer) => void;
}) {
  const { t } = useTranslation();
  const page = useVisibleItems(servers, 50);
  return (
    <IntegrationsSection
      title={t("integrations.connected")}
      onAdd={onAdd}
      empty={
        servers.length === 0 && (
          <EmptyState
            icon={<Server className="size-6" />}
            title={t("integrations.noServer")}
            hint={t("integrations.noServerHint")}
          />
        )
      }
      footer={
        <LoadMore
          loaded={page.loaded}
          total={page.total}
          hasMore={page.hasMore}
          isLoading={false}
          onLoad={page.loadMore}
        />
      }
    >
      {page.visible.map((server) => (
        <ServerCard key={server.name} server={server} onEdit={() => onEdit(server)} />
      ))}
    </IntegrationsSection>
  );
}
