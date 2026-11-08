import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { IntegrationCard } from "@/features/integrations/integration-card";
import { RevisedRemoveButton } from "@/components/shared/revised-remove-button";
import {
  integrationKeys,
  readMCPServer,
  useDeleteMCPServer,
  type MCPServer,
} from "@/features/integrations/api";
import { isStale } from "@/lib/api/revision";

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
            {/* The tools, the surface and the rulings live on their own
                page: three decisions with different consequences, and a
                dialog that disappears takes the record of what was chosen
                with it. */}
            <Button variant="ghost" size="sm" className="h-7" asChild>
              <Link to={`/integrations/mcp/${server.name}`}>
                {t("mcp.openServer")}
              </Link>
            </Button>
            <Button variant="ghost" size="sm" className="h-7" onClick={onEdit}>
              {t("agents.edit")}
            </Button>
            <RevisedRemoveButton
              title={t("integrations.removeServerTitle", { name: server.name })}
              description={t("integrations.removeServer")}
              revision={{
                queryKey: integrationKeys.mcpServer(server.name),
                read: async () => (await readMCPServer(server.name)).etag,
              }}
              onConfirm={(etag) =>
                remove.mutate(
                  { name: server.name, etag },
                  {
                    onSuccess: () =>
                      toast.success(
                        t("integrations.serverRemoved", { name: server.name }),
                      ),
                    onError: (e) =>
                      toast.error(
                        isStale(e)
                          ? t("revision.removeStale")
                          : e instanceof Error
                            ? e.message
                            : t("common.removeFailed"),
                      ),
                  },
                )
              }
            />
          </div>
        )
      }
    />
  );
}
