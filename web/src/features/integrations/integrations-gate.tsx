import { Plug } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Outlet } from "react-router-dom";
import { PAGE_ICONS } from "@/components/layout/nav";
import { EmptyState } from "@/components/shared/states";
import { PageHeader } from "@/components/shared/page-header";
import { useMe } from "@/features/session/api";

/**
 * The integrations screens, for those the server serves them to.
 *
 * Integrations are listed and configured at the platform's administration
 * scope only, so tool:read held in one area opens none of them. Following a
 * link there says why, rather than showing a page of requests answering 403.
 * An unknown platform list is a server that predates it, and opens as before.
 */
export function IntegrationsGate() {
  const { t } = useTranslation();
  const { data: me } = useMe();
  const platform = me?.platform;

  if (!platform || platform.includes("tool:read")) return <Outlet />;

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
