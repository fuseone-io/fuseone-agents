import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Eraser } from "lucide-react";
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
import { useEraseThread } from "@/features/workbench/conversation-api";
import { problemMessage } from "@/lib/api/problem-message";

/** Erases what was said in the person's own conversation, after asking. */
export function ConversationErase({ agentId, disabled }: { agentId: string; disabled: boolean }) {
  const { t } = useTranslation();
  const erase = useEraseThread(agentId);

  const confirm = () =>
    erase.mutate(undefined, {
      onSuccess: () => toast.success(t("workbench.conversation.erasedToast")),
      onError: (error) => toast.error(problemMessage(error, t)),
    });

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="ghost" size="sm" disabled={disabled || erase.isPending}>
          <Eraser className="size-4" aria-hidden />
          <span className="sr-only sm:not-sr-only">{t("workbench.conversation.erase")}</span>
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("workbench.conversation.eraseTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("workbench.conversation.eraseBody")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm}>{t("workbench.conversation.eraseConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
