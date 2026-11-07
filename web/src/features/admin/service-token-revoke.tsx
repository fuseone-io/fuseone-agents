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
import {
  useRevokeServiceToken,
  type ServiceToken,
} from "@/features/admin/service-accounts-api";
import { problemMessage } from "@/lib/api/problem-message";

/** Names the id as well as the name: after a rotation, two rows share a name. */
export function ServiceTokenRevoke({
  account,
  token,
  onClose,
}: {
  account: string;
  token: ServiceToken;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const revoke = useRevokeServiceToken();

  const confirm = () =>
    revoke.mutate(
      { account, token: token.id },
      {
        onSuccess: () => {
          toast.success(t("serviceAccounts.revoked"));
          onClose();
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("serviceAccounts.revokeTitle", { name: token.name })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("serviceAccounts.revokeExplains", { id: token.id })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={revoke.isPending}
            onClick={(event) => {
              event.preventDefault();
              confirm();
            }}
          >
            {t("serviceAccounts.revokeConfirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
