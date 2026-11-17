import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
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
import { useDiscardDraft } from "@/features/workbench/api";
import { problemMessage } from "@/lib/api/problem-message";

/** Discards the draft against the ETag the panel holds, after asking. */
export function WorkbenchDiscard({ agentId, etag }: { agentId: string; etag: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const discard = useDiscardDraft();

  const confirm = () =>
    discard.mutate(
      { agentId, etag },
      {
        onSuccess: () => {
          toast.success(t("workbench.discarded"));
          void navigate(`/agents/${agentId}/workbench`);
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" disabled={discard.isPending}>
          <Trash2 className="size-4" aria-hidden />
          {/* The icon alone on a phone, as the save beside it: in Portuguese
              the words pushed the header's own controls off the screen. */}
          <span className="sr-only sm:not-sr-only">{t("workbench.discard")}</span>
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("workbench.discardTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("workbench.discardBody")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm}>{t("workbench.discardConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
