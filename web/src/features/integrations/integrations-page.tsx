import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { problemMessage } from "@/lib/api/problem-message";
import { isStale } from "@/lib/api/revision";
import { useNavigate } from "react-router-dom";
import { useState } from "react";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Plug } from "lucide-react";
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
import { ServerEditor } from "@/features/integrations/server-editor";
import { ProviderEditor } from "@/features/integrations/provider-editor";
import { ConnectedServersSection } from "@/features/integrations/connected-servers-section";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";
import { ProviderCard } from "@/features/integrations/provider-card";
import {
  useIntegrations,
  useConnectorCatalog,
  useConnectorInstances,
  useDeleteConnectorInstance,
  useMCPUserCredentials,
  usePutConnectorInstance,
  type MCPServer,
  type ModelProvider,
} from "@/features/integrations/api";
import { ConnectorCatalogPanel } from "@/features/integrations/connectors/connector-catalog-panel";
import { useRecipes } from "@/features/integrations/mcp/api";
import { AvailableServersPanel } from "@/features/integrations/mcp/available-servers-panel";
import { UserCredentialsPanel } from "@/features/integrations/mcp/user-credentials-panel";
import { credentialServers } from "@/features/integrations/mcp/credential-server";
import { LoadMore } from "@/components/shared/load-more";
import { integrationAccess } from "@/features/integrations/integration-access";
import { useMe } from "@/features/session/api";
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
  const { data: me } = useMe();
  const access = integrationAccess(me);
  // Providers and channels are the platform's; MCP servers and connector
  // instances are their own scope's (NT-019). An area's administrator asks
  // for neither platform list, each of which would answer 403. The servers'
  // listing answers them with their own and what they inherit; the catalogue
  // and their own credentials are theirs to read.
  const integrations = useIntegrations(access.servers);
  const recipes = useRecipes(access.catalogue);
  const channels = useChannels(access.platform);
  const credentials = useMCPUserCredentials(access.credentials);
  // Asked whenever the connectors are offered, not only on their section:
  // the rail counts them from the moment the page opens.
  const connectorCatalog = useConnectorCatalog(access.connectors);
  // The instances carry hosts and databases, so they are listed to whoever
  // configures somewhere, and not to a reader of the platform (NT-019).
  const connectorInstances = useConnectorInstances(
    section === "connectors" && access.createConnector,
  );
  const putConnectorInstance = usePutConnectorInstance();
  const deleteConnectorInstance = useDeleteConnectorInstance();
  const [editing, setEditing] = useState<Editing | null>(null);

  const servers = integrations.data?.mcpServers ?? [];
  const providers = integrations.data?.providers ?? [];
  const providerPage = useVisibleItems(providers, 50);
  const available = availableEntries(listing(servers, recipes.data?.items ?? []));
  const counts = {
    connected: integrations.data ? servers.length : undefined,
    available:
      integrations.data && recipes.data ? available.length : undefined,
    credentials: credentials.data ? credentials.data.items.length : undefined,
    connectors: connectorCatalog.data
      ? connectorCatalog.data.items.length
      : undefined,
    providers: integrations.data ? providers.length : undefined,
    channels: channels.data ? channels.data.items.length : undefined,
  };
  // The credentials read the servers' listing only for whoever may list it.
  const needsIntegrations =
    section === "connected" ||
    section === "providers" ||
    (section === "credentials" && access.servers);

  // Where a server may be placed, for the sheet and the catalogue's form alike.
  return (
    <ScopeChoiceContext.Provider
      value={{ anyScope: access.anyScope, configure: access.configure }}
    >
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
        {/* Tool servers and providers are the platform's: an area's
            administrator is offered neither (NT-019). */}
        {access.platformConfigure && (
          <ConnectMenu
            onConnect={(kind) =>
              kind === "server"
                ? void navigate("/integrations/mcp")
                : setEditing({ kind, value: null })
            }
          />
        )}
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
          <ConnectedServersSection
            servers={servers}
            // Connecting starts at the catalogue, offered only to whoever may
            // create a server somewhere (NT-019).
            onAdd={
              access.createServer ? () => void navigate("/integrations/mcp") : undefined
            }
            onEdit={(server) => setEditing({ kind: "server", value: server })}
          />
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
            canCreate={access.createServer}
            servers={servers}
            recipes={recipes.data?.items ?? []}
            isLoading={integrations.isLoading || recipes.isLoading}
            error={integrations.error ?? recipes.error}
            onRetry={() => {
              void integrations.refetch();
              void recipes.refetch();
            }}
          />
        ) : section === "connectors" ? (
          <ConnectorCatalogPanel
            data={{
              connectors: connectorCatalog.data?.items ?? [],
              instances: connectorInstances.data?.items ?? [],
              catalogLoading: connectorCatalog.isLoading,
              instancesLoading: connectorInstances.isLoading,
              catalogError: connectorCatalog.error,
              instancesError: connectorInstances.error,
              canConfigure: access.createConnector,
              scope: { anyScope: access.anyScope, configure: access.configure },
            }}
            actions={{
              retryCatalog: () => void connectorCatalog.refetch(),
              retryInstances: () => void connectorInstances.refetch(),
              saveInstance: (input) => putConnectorInstance.mutateAsync(input),
              deleteInstance: (instance, etag) =>
                void deleteConnectorInstance.mutate(
                  { instance, etag },
                  {
                    onError: (error) =>
                      toast.error(
                        isStale(error)
                          ? t("revision.removeStale")
                          : problemMessage(error, t),
                      ),
                  },
                ),
            }}
          />
        ) : section === "credentials" ? (
          <UserCredentialsPanel
            servers={credentialServers(
              integrations.data?.mcpServers,
              credentials.data?.servers,
            )}
            recipes={recipes.data?.items ?? []}
            credentials={credentials.data?.items ?? []}
            isLoading={
              integrations.isLoading ||
              recipes.isLoading ||
              credentials.isLoading
            }
            error={integrations.error ?? recipes.error ?? credentials.error}
            // Asks again only what the caller may read: an area's author was
            // never asked for the servers' listing or the catalogue.
            onRetry={() => {
              if (access.servers) void integrations.refetch();
              if (access.catalogue) void recipes.refetch();
              void credentials.refetch();
            }}
          />
        ) : (
          <ChannelsTab />
        )}
      </IntegrationsShell>

      {editing?.kind === "server" && (
        <ServerEditor server={editing.value} onClose={() => setEditing(null)} />
      )}
      {editing?.kind === "provider" && (
        <ProviderEditor
          provider={editing.value}
          onClose={() => setEditing(null)}
        />
      )}
    </ScopeChoiceContext.Provider>
  );
}
