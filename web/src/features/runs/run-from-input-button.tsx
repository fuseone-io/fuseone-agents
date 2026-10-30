import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useRunFromInput } from "@/features/runs/run-from-input";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Opens a new run with the input this one was recorded with (#16).
 *
 * Not Replay, and the dialog says so: Replay re-checks recorded decisions and
 * executes nothing, while this calls real tools in a new run. A confirmation
 * because of that, and because what does not carry over — approvals, budget,
 * trust — is the part an operator needs to read before pressing.
 *
 * A refusal stays inside the dialog rather than in a toast. "The input was
 * erased" is the answer to the question the person just asked, and it should
 * be next to the button they pressed.
 */
export function RunFromInputButton({ runId }: { runId: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const rerun = useRunFromInput(runId);

  const confirm = () =>
    rerun.mutate(undefined, {
      onSuccess: (run) => {
        toast.success(t("runs.runFromInputOpened"), { description: run.runId });
        navigate(`/runs/${run.runId}`);
      },
    });

  return (
    <AlertDialog onOpenChange={() => rerun.reset()}>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" className="h-8">
          <RotateCcw className="size-4" aria-hidden />
          {t("runs.runFromInput")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("runs.runFromInputTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("runs.runFromInputHelp")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {rerun.error && (
          <p role="alert" className="text-sm text-danger">
            {problemMessage(rerun.error, t)}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          {/* A plain Button, not AlertDialogAction: the action closes the
              dialog on press, and a refusal must stay visible. */}
          <Button disabled={rerun.isPending} onClick={confirm}>
            {t("runs.runFromInputConfirm")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
