import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useState } from "react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Plug, Server } from "lucide-react";
import { ChannelsTab } from "@/features/channels/channels-tab";
import { useChannels } from "@/features/channels/api";
import { ConnectMenu } from "@/features/integrations/connect-menu";
import {
  IntegrationsShell,
  type IntegrationSection,
} from "@/features/integrations/integrations-shell";
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
  useMCPUserCredentials,
  type MCPServer,
  type ModelProvider,
} from "@/features/integrations/api";
import { useRecipes } from "@/features/integrations/mcp/api";
import { AvailableServersPanel } from "@/features/integrations/mcp/available-servers-panel";
import { UserCredentialsPanel } from "@/features/integrations/mcp/user-credentials-panel";
import { LoadMore } from "@/components/shared/load-more";
import { useVisibleItems } from "@/hooks/use-visible-items";
import {
  availableEntries,
  listing,
} from "@/features/integrations/mcp/catalogue";

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
export function IntegrationsPage({
  section = "connected",
}: {
  section?: IntegrationSection;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const integrations = useIntegrations();
  const recipes = useRecipes();
  const channels = useChannels();
  const credentials = useMCPUserCredentials();
  const [editing, setEditing] = useState<Editing | null>(null);

  const servers = integrations.data?.mcpServers ?? [];
  const providers = integrations.data?.providers ?? [];
  const serverPage = useVisibleItems(servers, 50);
  const providerPage = useVisibleItems(providers, 50);
  const available = availableEntries(listing(servers, recipes.data?.items ?? []));
  const counts = {
    connected: integrations.data ? servers.length : undefined,
    available:
      integrations.data && recipes.data ? available.length : undefined,
    credentials: credentials.data ? credentials.data.items.length : undefined,
    providers: integrations.data ? providers.length : undefined,
    channels: channels.data ? channels.data.items.length : undefined,
  };
  const needsIntegrations =
    section === "connected" ||
    section === "providers" ||
    section === "credentials";

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.integrations}
        title={t("nav.integrations")}
        description={t("integrations.subtitle")}
      >
        {/* Connecting a tool server starts at the catalogue: what this
            installation reaches and what it knows how to reach are one
            question, and answering it needs a page rather than a form.
            Editing one that exists stays a dialog — there is nothing to read
            there, only a field to correct. */}
        <ConnectMenu
          onConnect={(kind) =>
            kind === "server"
              ? void navigate("/integrations/mcp")
              : setEditing({ kind, value: null })
          }
        />
      </PageHeader>

      <IntegrationsShell active={section} counts={counts}>
        {needsIntegrations && integrations.isLoading ? (
          <LoadingRows rows={3} />
        ) : needsIntegrations && integrations.error ? (
          <ErrorState
            error={integrations.error}
            onRetry={() => void integrations.refetch()}
          />
        ) : section === "connected" ? (
          <>
            <IntegrationsSection
              title={t("integrations.connected")}
              onAdd={() => void navigate("/integrations/mcp")}
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
                  loaded={serverPage.loaded}
                  total={serverPage.total}
                  hasMore={serverPage.hasMore}
                  isLoading={false}
                  onLoad={serverPage.loadMore}
                />
              }
            >
              {serverPage.visible.map((server) => (
                <ServerCard
                  key={server.name}
                  server={server}
                  onEdit={() => setEditing({ kind: "server", value: server })}
                />
              ))}
            </IntegrationsSection>
          </>
        ) : section === "providers" ? (
          <>
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
              footer={
                <LoadMore
                  loaded={providerPage.loaded}
                  total={providerPage.total}
                  hasMore={providerPage.hasMore}
                  isLoading={false}
                  onLoad={providerPage.loadMore}
                />
              }
            >
              {providerPage.visible.map((p) => (
                <ProviderCard
                  key={p.name}
                  provider={p}
                  onEdit={() => setEditing({ kind: "provider", value: p })}
                />
              ))}
            </IntegrationsSection>
          </>
        ) : section === "available" ? (
          <AvailableServersPanel
            servers={servers}
            recipes={recipes.data?.items ?? []}
            isLoading={integrations.isLoading || recipes.isLoading}
            error={integrations.error ?? recipes.error}
            onRetry={() => {
              void integrations.refetch();
              void recipes.refetch();
            }}
          />
        ) : section === "credentials" ? (
          <UserCredentialsPanel
            servers={servers}
            recipes={recipes.data?.items ?? []}
            credentials={credentials.data?.items ?? []}
            isLoading={
              integrations.isLoading ||
              recipes.isLoading ||
              credentials.isLoading
            }
            error={integrations.error ?? recipes.error ?? credentials.error}
            onRetry={() => {
              void integrations.refetch();
              void recipes.refetch();
              void credentials.refetch();
            }}
          />
        ) : (
          <ChannelsTab />
        )}
      </IntegrationsShell>

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
