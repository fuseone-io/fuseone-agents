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
 * Providers and channels are listed and configured at the platform's
 * administration scope only. Connector instances and MCP servers are
 * configured at their own scope (NT-019), so an area's administrator opens
 * those screens, the catalogue they connect a server from and their own
 * credentials, and no other. Following a link elsewhere takes them to
 * what they have, rather than to a page of requests answering 403; one who
 * configures nowhere reads why. An unknown platform list is a server that
 * predates it, and opens as before.
 */
export function IntegrationsGate() {
  const { t } = useTranslation();
  const { data: me } = useMe();
  const { pathname } = useLocation();
  const access = integrationAccess(me);
  const own = ownScreen(pathname);

  // Until the session answers, open nothing: a screen opened on a guess asks
  // the platform for lists an area's administrator would be refused.
  if (!access.known) return <LoadingRows rows={3} />;
  if (access.platform || (own && access.servers)) return <Outlet />;
  // An area's administrator has the connected servers and the connectors.
  if (access.servers) return <Navigate to="/integrations" replace />;

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

/**
 * The screens an area's configurer opens: the connected servers, the
 * catalogue and a server's own page, the connectors, and their credentials.
 * Providers and channels are the platform's.
 */
function ownScreen(pathname: string) {
  const path = pathname.replace(/\/+$/, "");
  return (
    path === "/integrations" ||
    path === "/integrations/credentials" ||
    path.startsWith("/integrations/connectors") ||
    /^\/integrations\/mcp(\/[^/]+)?$/.test(path)
  );
}
