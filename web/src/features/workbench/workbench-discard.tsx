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
import { useDiscardDraft, useIsPublishing } from "@/features/workbench/api";
import { problemMessage } from "@/lib/api/problem-message";

/** Discards the draft against the ETag the panel holds, after asking. */
export function WorkbenchDiscard({ agentId, etag }: { agentId: string; etag: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const discard = useDiscardDraft();
  // Held while a publication is on its way: discarded under it, the draft the
  // person reads would no longer exist.
  const publishing = useIsPublishing(agentId);

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
        <Button variant="outline" size="sm" disabled={discard.isPending || publishing}>
          <Trash2 className="size-4" aria-hidden />
          {/* The icon alone on a phone, as the save beside it: in Portuguese
              the words pushed the header's own controls off the screen. */}
          {/* A secondary action: its words wait for the widest header. */}
          <span className="sr-only xl:not-sr-only">{t("workbench.discard")}</span>
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
