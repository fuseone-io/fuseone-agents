import { useState } from "react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Plug, Server } from "lucide-react";
import { ConnectMenu } from "@/features/integrations/connect-menu";
import { IntegrationsSection } from "@/features/integrations/integrations-section";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { ServerForm } from "@/features/integrations/server-form";
import { ProviderForm } from "@/features/integrations/provider-form";
import { ServerCard } from "@/features/integrations/server-card";
import { ProviderCard } from "@/features/integrations/provider-card";
import {
  useIntegrations,
  type MCPServer,
  type ModelProvider,
} from "@/features/integrations/api";

type Editing =
  | { kind: "server"; value: MCPServer | null }
  | { kind: "provider"; value: ModelProvider | null };

/**
 * What the platform is connected to, and whether any of it is answering.
 *
 * Its own screen rather than a tab under Administração: everything an agent
 * can reach outside this installation is here, and burying it three clicks in
 * made "what are we connected to" a question nobody could answer at a glance.
 *
 * Cards rather than rows because the state of a connection is the point, and a
 * table makes every system look equally fine until somebody reads the column
 * on the right.
 */
export function IntegrationsPage() {
  const { data, isLoading, error, refetch } = useIntegrations();
  const [editing, setEditing] = useState<Editing | null>(null);

  const servers = data?.mcpServers ?? [];
  const providers = data?.providers ?? [];

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.integrations}
        title="Integrações"
        description="Tudo que os agentes alcançam fora desta instalação, e se está respondendo."
      >
        <ConnectMenu onConnect={(kind) => setEditing({ kind, value: null })} />
      </PageHeader>

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : (
        <div className="flex flex-col gap-6">
          <IntegrationsSection
            title="Servidores de ferramentas"
            onAdd={() => setEditing({ kind: "server", value: null })}
            empty={
              servers.length === 0 && (
                <EmptyState
                  icon={<Server className="size-6" />}
                  title="Nenhum servidor configurado"
                  hint="Um servidor MCP é o que dá ferramentas aos agentes. Enquanto não houver um, os agentes só conseguem raciocinar."
                />
              )
            }
          >
            {servers.map((server) => (
              <ServerCard
                key={server.name}
                server={server}
                onEdit={() => setEditing({ kind: "server", value: server })}
              />
            ))}
          </IntegrationsSection>

          <IntegrationsSection
            title="Provedores de modelo"
            onAdd={() => setEditing({ kind: "provider", value: null })}
            empty={
              providers.length === 0 && (
                <EmptyState
                  icon={<Plug className="size-6" />}
                  title="Nenhum provedor configurado"
                  hint="Sem provedor, nenhuma execução avança: o agente não tem com o que planejar."
                />
              )
            }
          >
            {providers.map((provider) => (
              <ProviderCard
                key={provider.name}
                provider={provider}
                onEdit={() => setEditing({ kind: "provider", value: provider })}
              />
            ))}
          </IntegrationsSection>

          {editing?.kind === "server" && (
            <ServerForm
              server={editing.value}
              onClose={() => setEditing(null)}
            />
          )}
          {editing?.kind === "provider" && (
            <ProviderForm
              provider={editing.value}
              onClose={() => setEditing(null)}
            />
          )}
        </div>
      )}
    </>
  );
}
