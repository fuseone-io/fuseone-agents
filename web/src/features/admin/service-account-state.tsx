import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { ServiceAccountSection } from "@/features/admin/service-account-section";
import {
  useServiceTokens,
  useSetServiceAccountDisabled,
  type ServiceAccount,
} from "@/features/admin/service-accounts-api";
import { liveTokens } from "@/features/admin/service-token-state";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Both directions ask. Disabling stops whatever is running on it; enabling
 * hands back every token still valid, all at once, which is the one act on this
 * screen that gives access rather than takes it, and so says how much.
 */
export function ServiceAccountState({ account }: { account: ServiceAccount }) {
  const { t } = useTranslation();
  const [asking, setAsking] = useState(false);
  const set = useSetServiceAccountDisabled();
  const { data } = useServiceTokens(account.id);
  const back = liveTokens(data?.items ?? []).length;
  const verb = account.disabled ? "enable" : "disable";

  const confirm = () =>
    set.mutate(
      { account: account.id, disabled: !account.disabled },
      {
        onSuccess: () => toast.success(t(`serviceAccounts.${verb}d`)),
        onError: (error) => toast.error(problemMessage(error, t)),
        onSettled: () => setAsking(false),
      },
    );

  return (
    <ServiceAccountSection
      title={t("serviceAccounts.stateTitle")}
      explains={t("serviceAccounts.stateExplains")}
    >
      <div>
        <Button variant="outline" size="sm" onClick={() => setAsking(true)}>
          {t(`serviceAccounts.${verb}`)}
        </Button>
      </div>
      <AlertDialog open={asking} onOpenChange={setAsking}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(`serviceAccounts.${verb}Title`, { name: account.name })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {account.disabled
                ? t("serviceAccounts.enableExplains", { count: back })
                : t("serviceAccounts.disableExplains")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant={account.disabled ? "default" : "destructive"}
              disabled={set.isPending}
              onClick={(event) => {
                event.preventDefault();
                confirm();
              }}
            >
              {t(`serviceAccounts.${verb}Confirm`)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </ServiceAccountSection>
  );
}
