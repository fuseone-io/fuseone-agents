import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import type { RemoteAuthPlan } from "@/features/integrations/mcp/auth-plan";

/** What the recipe says of how the server authenticates, when it is worth saying. */
export function RemoteAuthSummary({ plan }: { plan: RemoteAuthPlan }) {
  const { t } = useTranslation();
  if (!plan.known) {
    return (
      <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
        {t("mcp.authUnknownShape")}
      </p>
    );
  }

  if (plan.noAuth !== null && plan.secret === null && plan.oauth === null) {
    return (
      <p className="rounded-lg border bg-muted px-3 py-2 text-xs text-muted-foreground">
        {t("mcp.authNoCredential")}
      </p>
    );
  }

  if (plan.unsupported.length === 0) return null;
  const modes = plan.unsupported
    .map((mode) => mode.label ?? t(`mcp.authMode.${mode.type}`))
    .join(", ");
  return (
    <p className="rounded-lg border border-warning/30 bg-warning-surface px-3 py-2 text-xs text-warning">
      {t("mcp.authShapeUnsupported", { modes })}
    </p>
  );
}

export function ConnectionRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2">
      <dt className="w-28 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate">
        <Mono className="text-xs">{value}</Mono>
      </dd>
    </div>
  );
}
