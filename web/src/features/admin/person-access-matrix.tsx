import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { ScopeGrant } from "@/features/admin/person-access-model";
import { PersonGrantTable } from "@/features/admin/person-grant-table";

export function PersonAccessMatrix({
  groups,
  onEdit,
}: {
  groups: ScopeGrant[];
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const providerScopes = groups.filter((group) => group.origin === "provider");
  const matrixNote =
    groups.length === 0
      ? t("people.matrixEmpty")
      : [
          t("people.scopeCount", { count: groups.length }),
          providerScopes.length > 0
            ? t("people.providerScopeCount", { count: providerScopes.length })
            : undefined,
        ]
          .filter(Boolean)
          .join(" · ");

  return (
    <div className="flex flex-col gap-3 bg-surface-sunken px-4 pb-4 pt-3 lg:pl-16">
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-2xs font-semibold uppercase tracking-label text-text-disabled">
          {t("people.rolesByScope")}
        </span>
        <span className="text-xs text-muted-foreground">{matrixNote}</span>
      </div>

      <div className="max-w-[720px] overflow-hidden rounded-lg border bg-card">
        {groups.length === 0 ? (
          <div className="p-4 text-sm text-muted-foreground">
            {t("people.matrixEmpty")}
          </div>
        ) : (
          <PersonGrantTable groups={groups} />
        )}
        <div className="flex flex-wrap items-center gap-2 border-t border-border-subtle p-3">
          <Button
            variant="outline"
            size="xs"
            className="border-dashed"
            onClick={onEdit}
          >
            <Plus className="size-3.5" aria-hidden />
            {t("people.addScope")}
          </Button>
          <span className="text-xs text-text-disabled">
            {t("people.providerScopesReadOnly")}
          </span>
        </div>
      </div>
    </div>
  );
}
