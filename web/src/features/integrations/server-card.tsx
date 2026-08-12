import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { IntegrationCard } from "@/features/integrations/integration-card";
import { RemoveButton } from "@/components/shared/remove-button";
import {
  useDeleteMCPServer,
  type MCPServer,
} from "@/features/integrations/api";

/** A tool server: what it runs, and whether it answered. */
export function ServerCard({
  server,
  onEdit,
}: {
  server: MCPServer;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const remove = useDeleteMCPServer();

  return (
    <IntegrationCard
      name={server.name}
      kind={t("integrations.mcpServer")}
      description={
        server.managed === false
          ? t("integrations.serverFromFlag")
          : server.transport === "http"
            ? (server.url ?? "")
            : [server.command, ...(server.args ?? [])].join(" ")
      }
      enabled={server.enabled}
      health={server.health}
      action={
        // A server nobody configured here cannot be edited or removed here
        // either. Offering the buttons would promise something the console
        // cannot do.
        server.managed === false ? undefined : (
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="sm" className="h-7" onClick={onEdit}>
              {t("agents.edit")}
            </Button>
            <RemoveButton
              title={t("integrations.removeServerTitle", { name: server.name })}
              description={t("integrations.removeServer")}
              onConfirm={() =>
                remove.mutate(server.name, {
                  onSuccess: () =>
                    toast.success(
                      t("integrations.serverRemoved", { name: server.name }),
                    ),
                  onError: (e) =>
                    toast.error(
                      e instanceof Error ? e.message : t("common.removeFailed"),
                    ),
                })
              }
            />
          </div>
        )
      }
    />
  );
}
