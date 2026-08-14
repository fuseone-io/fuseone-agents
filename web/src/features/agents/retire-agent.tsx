import { useNavigate } from "react-router-dom";
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
import { useSetAgentRetired } from "@/features/agents/agent-editor-api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Taking an agent out of circulation.
 *
 * Never a delete, and the dialog says why rather than only asking twice: a run
 * is pinned to a version and that version is the only correct explanation of
 * what the run did. Somebody who came looking for a delete button deserves to
 * find out here that what they want is this, and what it keeps.
 */
export function RetireDialog({
  agentId,
  onClose,
}: {
  agentId: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const set = useSetAgentRetired(agentId);

  return (
    <AlertDialog open onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("agents.retireTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("agents.retireExplains")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() =>
              set.mutate(true, {
                onSuccess: () => {
                  toast.success(t("agents.retired"));
                  void navigate("/agents");
                },
                onError: (error) => toast.error(problemMessage(error, t)),
              })
            }
          >
            {t("agents.retire")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
