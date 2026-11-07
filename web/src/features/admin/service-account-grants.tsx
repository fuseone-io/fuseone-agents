import { useTranslation } from "react-i18next";
import { GrantBadge } from "@/features/admin/grant-badge";
import type { ServiceAccount } from "@/features/admin/service-accounts-api";

export function ServiceAccountGrants({ account }: { account: ServiceAccount }) {
  const { t } = useTranslation();
  if (account.grants.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        {t("serviceAccounts.holdsNothing")}
      </p>
    );
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {account.grants.map((grant) => (
        <GrantBadge
          key={`${grant.company}/${grant.area}/${grant.role}`}
          grant={{ ...grant, asserted: false }}
        />
      ))}
    </div>
  );
}
