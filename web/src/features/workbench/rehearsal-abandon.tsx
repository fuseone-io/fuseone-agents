import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { CircleStop } from "lucide-react";
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
import { useAbandonRehearsal } from "@/features/workbench/rehearsal-api";
import { problemMessage } from "@/lib/api/problem-message";

/** Ends the rehearsal wherever it stands, after asking. Nothing is undone: nothing acted. */
export function RehearsalAbandon({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const abandon = useAbandonRehearsal(runId);

  const confirm = () =>
    abandon.mutate(undefined, { onError: (error) => toast.error(problemMessage(error, t)) });

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" disabled={abandon.isPending}>
          <CircleStop className="size-4" aria-hidden />
          {t("workbench.rehearsal.abandon")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("workbench.rehearsal.abandonTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("workbench.rehearsal.abandonBody")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm}>{t("workbench.rehearsal.abandonConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
