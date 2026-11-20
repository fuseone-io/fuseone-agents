import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Power } from "lucide-react";
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
import { Mono } from "@/components/shared/mono";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { eventCycleMessage } from "@/features/agents/event-cycle-warning";
import { useActivateCandidate, useAgentHead } from "@/features/workbench/candidates-api";
import { currentLocale } from "@/i18n";
import { problemMessage } from "@/lib/api/problem-message";
import type { Candidate } from "@/lib/api/client";

/**
 * Activating a candidate (NT-018 phase 2): its sealed text, shown whole, goes live in
 * place of the agent's current version and is kept as every published version is. The
 * activation names the agent as this dialog read it, so a version published meanwhile is
 * refused rather than replaced unseen.
 */
export function CandidateActivate({ agentId, candidate, onActivated }: {
  agentId: string;
  candidate: Candidate;
  onActivated: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const head = useAgentHead(agentId, open);
  const activate = useActivateCandidate(agentId);

  const confirm = () => {
    if (head.data === undefined) return;
    activate.mutate({ version: candidate.version, etag: head.data?.etag ?? null }, {
      onSuccess: (done) => {
        toast.success(t(done.paused ? "workbench.candidates.activatedPaused" : "workbench.candidates.activatedLive", { version: done.versionId }));
        const cycle = eventCycleMessage(done.eventCycleWarning, t, currentLocale());
        if (cycle) toast.warning(cycle, { duration: Infinity });
        onActivated();
      },
      onError: (error) => toast.error(problemMessage(error, t)),
    });
  };

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogTrigger asChild>
        <Button><Power className="size-4" aria-hidden />{t("workbench.candidates.activate")}</Button>
      </AlertDialogTrigger>
      <AlertDialogContent className="sm:max-w-2xl">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("workbench.candidates.activateTitle", { version: candidate.version })}</AlertDialogTitle>
          {/* A div, since the placeholder is a block while the agent is read. */}
          <AlertDialogDescription asChild>
            <div>
            {head.isLoading ? <Skeleton className="h-4 w-2/3" /> : replaces(t, head.data)}{" "}
            {t("workbench.candidates.activateKept")}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <ScrollArea className="max-h-[50vh] rounded-md border bg-muted">
          <Mono className="block whitespace-pre-wrap wrap-anywhere p-3 text-xs">{candidate.definition}</Mono>
        </ScrollArea>
        {head.error && <p className="text-sm text-danger">{problemMessage(head.error, t)}</p>}
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm} disabled={head.data === undefined || activate.isPending}>
            {t("workbench.candidates.activateConfirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function replaces(t: ReturnType<typeof useTranslation>["t"], head: { versionId: string } | null | undefined): string {
  if (head === null) return t("workbench.candidates.activateFirst");
  return head ? t("workbench.candidates.activateReplaces", { current: head.versionId }) : "";
}
