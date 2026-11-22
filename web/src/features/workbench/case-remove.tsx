import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Trash2 } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { useDeleteRegression } from "@/features/agents/regressions-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Removes a case from the corpus, after asking. Said plainly what it is (NT-018 phase 5b,
 * D1): a new version of the corpus without the case, with nothing erased, because earlier
 * versions still name it.
 */
export function CaseRemove({ agentId, caseId }: { agentId: string; caseId: string }) {
  const { t } = useTranslation();
  const remove = useDeleteRegression(agentId);

  const confirm = () =>
    remove.mutate(caseId, {
      onSuccess: () => toast.success(t("workbench.cases.removed")),
      onError: (error) => toast.error(problemMessage(error, t)),
    });

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="ghost" size="icon" disabled={remove.isPending} aria-label={t("workbench.cases.remove", { id: caseId })}>
          <Trash2 className="size-4" aria-hidden />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("workbench.cases.removeTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("workbench.cases.removeBody")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm}>{t("workbench.cases.removeConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
