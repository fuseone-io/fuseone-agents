import { useTranslation } from "react-i18next";
import { ArrowLeft } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { ServiceAccountAccess } from "@/features/admin/service-account-access";
import { ServiceAccountState } from "@/features/admin/service-account-state";
import { ServiceAccountStatus } from "@/features/admin/service-account-status";
import { ServiceTokens } from "@/features/admin/service-tokens";
import type { ServiceAccount } from "@/features/admin/service-accounts-api";

/**
 * Three different acts, in three different places, because they are easy to
 * confuse and mean different things: revoking ends one credential for good,
 * disabling stops all of them and can be undone, and access says what any of
 * them may do. A single "remove" would leave somebody guessing which they did.
 */
export function ServiceAccountDetail({
  account,
  onBack,
}: {
  account: ServiceAccount;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft className="size-4" aria-hidden />
          {t("serviceAccounts.back")}
        </Button>
        <Mono>{account.name}</Mono>
        <ServiceAccountStatus disabled={account.disabled} />
      </div>
      {account.description && (
        <p className="text-sm text-muted-foreground">{account.description}</p>
      )}
      <ServiceTokens account={account} />
      <ServiceAccountAccess account={account} />
      <ServiceAccountState account={account} />
      <p className="text-xs text-muted-foreground">
        {t("serviceAccounts.trailNote")}
      </p>
    </div>
  );
}
