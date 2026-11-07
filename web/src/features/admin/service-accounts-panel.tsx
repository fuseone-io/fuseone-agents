import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Cpu, Plus } from "lucide-react";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { Button } from "@/components/ui/button";
import { ServiceAccountDetail } from "@/features/admin/service-account-detail";
import { ServiceAccountForm } from "@/features/admin/service-account-form";
import { ServiceAccountRow } from "@/features/admin/service-account-row";
import { useServiceAccounts } from "@/features/admin/service-accounts-api";

/**
 * The machines that call this installation.
 *
 * Opening one is by id and not by a copy of the row, so what its screen says
 * about it is what the server last answered, whichever act changed it.
 */
export function ServiceAccountsPanel() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useServiceAccounts();
  const [openId, setOpenId] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const accounts = data?.items ?? [];
  const open = accounts.find((account) => account.id === openId);

  if (open) {
    return (
      <ServiceAccountDetail account={open} onBack={() => setOpenId(null)} />
    );
  }

  return (
    <section className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <header className="flex flex-wrap items-start gap-3 border-b px-4 py-3.5">
        <div className="min-w-0 flex-1">
          <h2 className="text-base font-medium">{t("serviceAccounts.title")}</h2>
          <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
            {t("serviceAccounts.subtitle")}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
          <Plus className="size-4" aria-hidden />
          {t("serviceAccounts.create")}
        </Button>
      </header>

      {isLoading ? (
        <div className="p-4">
          <LoadingRows rows={3} />
        </div>
      ) : error ? (
        <div className="p-4">
          <ErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : accounts.length === 0 ? (
        <div className="p-4">
          <EmptyState
            icon={<Cpu className="size-6" />}
            title={t("serviceAccounts.emptyTitle")}
            hint={t("serviceAccounts.emptyHint")}
          />
        </div>
      ) : (
        <ul className="divide-y divide-border-subtle">
          {accounts.map((account) => (
            <li key={account.id}>
              <ServiceAccountRow
                account={account}
                onManage={() => setOpenId(account.id)}
              />
            </li>
          ))}
        </ul>
      )}

      {adding && <ServiceAccountForm onClose={() => setAdding(false)} />}
    </section>
  );
}
