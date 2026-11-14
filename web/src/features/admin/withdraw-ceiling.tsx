import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useWithdrawPlatformRuling } from "@/features/admin/api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Withdrawing the platform's ceiling on one tool, the platform curator's alone.
 *
 * Confirmed, because what follows is not undone by the button: the owner's
 * ruling takes over, and with none the tool stops executing.
 */
export function WithdrawCeiling({ toolId }: { toolId: string }) {
  const { t } = useTranslation();
  const withdraw = useWithdrawPlatformRuling();

  async function confirm() {
    try {
      await withdraw.mutateAsync(toolId);
      toast.success(t("admin.ceilingWithdrawn", { tool: toolId }));
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" disabled={withdraw.isPending}>
          {t("admin.withdrawCeiling")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("admin.withdrawCeilingTitle", { tool: toolId })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("admin.withdrawCeilingBody")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={() => void confirm()}>
            {t("admin.withdraw")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
