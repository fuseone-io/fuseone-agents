import { Plug } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { PAGE_ICONS } from "@/components/layout/nav";
import { EmptyState, LoadingRows } from "@/components/shared/states";
import { PageHeader } from "@/components/shared/page-header";
import { integrationAccess } from "@/features/integrations/integration-access";
import { useMe } from "@/features/session/api";

/**
 * The integrations screens, for those the server serves them to.
 *
 * MCP servers, providers and channels are listed and configured at the
 * platform's administration scope only. Connector instances are configured at
 * their own scope (NT-019), so an area's administrator opens that screen and
 * no other. Following a link elsewhere says why, rather than showing a page of
 * requests answering 403. An unknown platform list is a server that predates
 * it, and opens as before.
 */
export function IntegrationsGate() {
  const { t } = useTranslation();
  const { data: me } = useMe();
  const { pathname } = useLocation();
  const access = integrationAccess(me);
  const connectors = pathname.startsWith("/integrations/connectors");

  // Until the session answers, open nothing: a screen opened on a guess asks
  // the platform for lists an area's administrator would be refused.
  if (!access.known) return <LoadingRows rows={3} />;
  if (access.platform || (connectors && access.connectors)) return <Outlet />;
  // An area's administrator has one integrations screen: take them to it.
  if (access.connectors) return <Navigate to="/integrations/connectors" replace />;

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.integrations}
        title={t("nav.integrations")}
        description={t("integrations.subtitle")}
      />
      <EmptyState
        icon={<Plug className="size-6" />}
        title={t("integrations.platformOnly")}
        hint={t("integrations.platformOnlyHint")}
      />
    </>
  );
}
