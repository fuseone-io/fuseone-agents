import { useTranslation } from "react-i18next";
import { PersonAccessChip } from "@/features/admin/person-access-chip";
import type { ScopeGrant } from "@/features/admin/person-access-model";

export function PersonAccessSummary({ groups }: { groups: ScopeGrant[] }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0">
      <MobileColumnLabel>{t("people.access")}</MobileColumnLabel>
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        {groups.length === 0 ? (
          <span className="text-xs text-text-disabled">
            {t("people.noRoleGranted")}
          </span>
        ) : (
          groups.map((group) => (
            <PersonAccessChip key={group.scope} group={group} />
          ))
        )}
      </div>
    </div>
  );
}

export function MobileColumnLabel({ children }: { children: string }) {
  return (
    <p className="mb-1 text-2xs font-semibold uppercase tracking-label text-text-disabled lg:hidden">
      {children}
    </p>
  );
}
