import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { useIntegrations } from "@/features/integrations/api";
import { AvailableServersPanel } from "@/features/integrations/mcp/available-servers-panel";
import { useRecipes } from "@/features/integrations/mcp/api";
import { useCatalogueTools } from "@/features/integrations/mcp/use-catalogue-tools";
import { useTranslation } from "react-i18next";
import { integrationAccess } from "@/features/integrations/integration-access";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";
import { useMe } from "@/features/session/api";

export function CataloguePage() {
  const { t } = useTranslation();
  const integrations = useIntegrations();
  const recipes = useRecipes();
  const access = integrationAccess(useMe().data);
  const servers = integrations.data?.mcpServers ?? [];
  // Asked by the scope of each server shown, so an area's server lists its
  // tools to an area's reader (NT-019).
  const tools = useCatalogueTools(servers, access.platform);

  // Only whoever may create a server is offered to connect one, and only
  // where they may place it (NT-019).
  return (
    <ScopeChoiceContext.Provider
      value={{ anyScope: access.anyScope, configure: access.configure }}
    >
      <PageHeader
        icon={PAGE_ICONS.integrations}
        title={t("mcp.catalogue")}
        description={t("mcp.catalogueDescription")}
      />
      <AvailableServersPanel
        canCreate={access.createServer}
        servers={servers}
        recipes={recipes.data?.items ?? []}
        tools={tools.data?.items ?? []}
        isLoading={integrations.isLoading || recipes.isLoading || tools.isLoading}
        error={integrations.error ?? recipes.error ?? tools.error}
        onRetry={() => {
          void integrations.refetch();
          void recipes.refetch();
          tools.refetch();
        }}
      />
    </ScopeChoiceContext.Provider>
  );
}
