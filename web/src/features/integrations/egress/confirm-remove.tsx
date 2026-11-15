import { useTranslation } from "react-i18next";
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
import { scopeLabel, type EgressPolicy } from "@/features/integrations/egress/api";

/** Removing a list is not denying: an area falls back to its company's, or nothing. */
export function ConfirmRemove({
  policy,
  onCancel,
  onConfirm,
}: {
  policy: EgressPolicy | null;
  onCancel: () => void;
  onConfirm: (policy: EgressPolicy) => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={policy !== null} onOpenChange={(open) => !open && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("integrations.egress.removeTitle", { scope: policy ? scopeLabel(policy) : "" })}</AlertDialogTitle>
          <AlertDialogDescription>{t("integrations.egress.removeHint")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={() => policy && onConfirm(policy)}>{t("integrations.egress.remove")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
