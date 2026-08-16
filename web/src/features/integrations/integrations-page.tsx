import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useState } from "react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Plug, Server } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ChannelsTab } from "@/features/channels/channels-tab";
import { useTab } from "@/features/preferences/use-preferences";
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
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data, isLoading, error, refetch } = useIntegrations();
  const [editing, setEditing] = useState<Editing | null>(null);
  const tab = useTab("integrations", "servers");

  const servers = data?.mcpServers ?? [];
  const providers = data?.providers ?? [];

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.integrations}
        title={t("nav.integrations")}
        description={t("integrations.subtitle")}
      >
        {/* Connecting a tool server is a page of its own: the catalogue is
            read before anything is decided, and reading needs somewhere to
            stand. Editing one that exists stays a dialog — there is nothing
            to read, only a field to correct. */}
        <ConnectMenu
          onConnect={(kind) =>
            kind === "server"
              ? void navigate("/integrations/mcp/new")
              : setEditing({ kind, value: null })
          }
        />
      </PageHeader>

      {isLoading ? (
        <LoadingRows rows={3} />
      ) : error ? (
        <ErrorState error={error} onRetry={() => void refetch()} />
      ) : (
        // Two tabs rather than two stacked sections: connecting a tool server
        // and connecting a model are different jobs, done by the same person
        // on different days. The cost is that the page no longer answers
        // "what are we connected to" in one glance, so each tab says how many
        // it holds without being opened.
        <Tabs {...tab}>
          <TabsList>
            <TabsTrigger value="servers">
              {t("integrations.servers")}
              <span className="ml-1.5 font-mono text-2xs tabular-nums opacity-60">
                {servers.length}
              </span>
            </TabsTrigger>
            <TabsTrigger value="providers">
              {t("integrations.providers")}
              <span className="ml-1.5 font-mono text-2xs tabular-nums opacity-60">
                {providers.length}
              </span>
            </TabsTrigger>
            {/* Same job — what this installation is connected to — and unlike
                the other two, nothing here grants an agent any ability. */}
            <TabsTrigger value="channels">{t("channels.channels")}</TabsTrigger>
          </TabsList>

          <TabsContent value="servers" className="mt-4">
            <IntegrationsSection
              title={t("integrations.servers")}
              onAdd={() => void navigate("/integrations/mcp/new")}
              empty={
                servers.length === 0 && (
                  <EmptyState
                    icon={<Server className="size-6" />}
                    title={t("integrations.noServer")}
                    hint={t("integrations.noServerHint")}
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
          </TabsContent>

          <TabsContent value="channels" className="mt-4">
            <ChannelsTab />
          </TabsContent>

          <TabsContent value="providers" className="mt-4">
            <IntegrationsSection
              title={t("integrations.providers")}
              onAdd={() => setEditing({ kind: "provider", value: null })}
              empty={
                providers.length === 0 && (
                  <EmptyState
                    icon={<Plug className="size-6" />}
                    title={t("integrations.noProvider")}
                    hint={t("integrations.noProviderHint")}
                  />
                )
              }
            >
              {providers.map((p) => (
                <ProviderCard
                  key={p.name}
                  provider={p}
                  onEdit={() => setEditing({ kind: "provider", value: p })}
                />
              ))}
            </IntegrationsSection>
          </TabsContent>
        </Tabs>
      )}

      {editing?.kind === "server" && (
        <ServerForm server={editing.value} onClose={() => setEditing(null)} />
      )}
      {editing?.kind === "provider" && (
        <ProviderForm
          provider={editing.value}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  );
}
