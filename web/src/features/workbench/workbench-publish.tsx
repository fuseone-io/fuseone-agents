import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Rocket } from "lucide-react";
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
import { useQueryClient } from "@tanstack/react-query";
import { isCurrentSnapshot, usePublishDraft, workbenchKeys, type RevisedDraft } from "@/features/workbench/api";
import { eventCycleMessage } from "@/features/agents/event-cycle-warning";
import { currentLocale } from "@/i18n";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Publishes the saved draft as the agent's next version (NT-018, contracts 4
 * and 10), after asking. The primary action once there is nothing to save:
 * what is published is the draft as stored, never unsaved edits.
 */
export function WorkbenchPublish({
  agentId,
  draft,
  unsaved,
  onPublished,
}: {
  agentId: string;
  draft: RevisedDraft;
  /** Edits not saved yet: publishing waits for them. */
  unsaved: boolean;
  onPublished: (draft: RevisedDraft) => void;
}) {
  const { t } = useTranslation();
  const publishing = usePublishDraft(agentId);
  const queryClient = useQueryClient();

  const confirm = () =>
    publishing.mutate(draft.etag, {
      onSuccess: (published) => {
        // Taken in only when nothing newer was written while it answered: the
        // panel already holds that newer draft, and its own base was moved.
        if (isCurrentSnapshot(queryClient.getQueryData(workbenchKeys.draft(agentId)), published)) {
          onPublished(published.draft);
        }
        toast.success(t(published.paused ? "workbench.publishedPaused" : "workbench.publishedLive", { version: published.versionId }));
        // A second toast, kept until dismissed: the version was written either
        // way, and a loop it closes deserves to be read, not to fade (#227).
        const cycle = eventCycleMessage(published.eventCycleWarning, t, currentLocale());
        if (cycle) toast.warning(cycle, { duration: Infinity });
      },
      onError: (error) => toast.error(problemMessage(error, t)),
    });

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="sm" variant={unsaved ? "outline" : "default"} disabled={unsaved || publishing.isPending}>
          <Rocket className="size-4" aria-hidden />
          <span className="sr-only sm:not-sr-only">{t("workbench.publish")}</span>
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("workbench.publishTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("workbench.publishBody")}</AlertDialogDescription>
          {/* Publishing here skips what a candidate is for; the person should know before saying yes. */}
          <p className="text-sm font-medium text-foreground">{t("workbench.publishSkipsTesting")}</p>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm}>{t("workbench.publishConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
