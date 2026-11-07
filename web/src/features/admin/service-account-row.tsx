import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { ServiceAccountGrants } from "@/features/admin/service-account-grants";
import { ServiceAccountStatus } from "@/features/admin/service-account-status";
import type { ServiceAccount } from "@/features/admin/service-accounts-api";

export function ServiceAccountRow({
  account,
  onManage,
}: {
  account: ServiceAccount;
  onManage: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-3 px-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex flex-wrap items-center gap-2">
          <Mono>{account.name}</Mono>
          <ServiceAccountStatus disabled={account.disabled} />
        </div>
        {account.description && (
          <p className="text-xs text-muted-foreground">{account.description}</p>
        )}
        <ServiceAccountGrants account={account} />
      </div>
      <Button
        variant="outline"
        size="sm"
        aria-label={t("serviceAccounts.manageNamed", { name: account.name })}
        onClick={onManage}
      >
        {t("serviceAccounts.manage")}
      </Button>
    </div>
  );
}
