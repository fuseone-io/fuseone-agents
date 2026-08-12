import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { RemoveButton } from "@/components/shared/remove-button";
import type { IdentityProvider } from "@/features/admin/identity-api";

/**
 * One provider, and what it grants.
 *
 * The mapping count is on the row rather than behind the edit screen because
 * zero is the state worth catching from across the page: people sign in
 * successfully and can do nothing, which reads as a broken product rather than
 * as configuration nobody finished.
 */
export function IdentityProviderRow({
  provider,
  onEdit,
  onRemove,
}: {
  provider: IdentityProvider;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const mappings = provider.mappings ?? [];

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-lg border p-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-medium">{provider.display || provider.id}</span>
          {!provider.enabled && (
            <Badge variant="outline">{t("identity.disabled")}</Badge>
          )}
          {mappings.length === 0 && (
            <Badge variant="destructive">{t("identity.grantsNothing")}</Badge>
          )}
        </div>
        <Mono dim className="truncate text-xs">
          {provider.issuer}
        </Mono>
      </div>

      <span className="text-xs text-muted-foreground">
        {t("identity.mappingCount", { count: mappings.length })}
      </span>
      {!provider.hasSecret && (
        <Badge variant="outline">{t("identity.noSecret")}</Badge>
      )}

      <div className="flex items-center gap-1">
        <Button variant="ghost" size="sm" className="h-7" onClick={onEdit}>
          {t("agents.edit")}
        </Button>
        <RemoveButton
          title={t("identity.removeTitle", { name: provider.id })}
          description={t("identity.removeDescription")}
          onConfirm={onRemove}
        />
      </div>
    </div>
  );
}
