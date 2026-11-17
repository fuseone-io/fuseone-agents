import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { useAcceptProposal, useRejectProposal } from "@/features/workbench/conversation-api";
import type { Decide } from "@/features/workbench/proposal-card";
import type { RevisedDraft } from "@/features/workbench/api";
import { problemMessage } from "@/lib/api/problem-message";
import { isStale } from "@/lib/api/revision";

/**
 * Accepting and rejecting the model's proposals. An accepted one answers the
 * draft it made, which the panel takes in under the person's unsaved edits;
 * one made over an older draft is refused, and says so.
 */
export function useDecide(agentId: string, onDraft: (draft: RevisedDraft) => void, reread: () => void): Decide {
  const { t } = useTranslation();
  const accepting = useAcceptProposal(agentId);
  const rejecting = useRejectProposal(agentId);
  const refused = (error: unknown) =>
    toast.error(isStale(error) ? t("workbench.conversation.staleProposal") : problemMessage(error, t));

  return {
    busy: accepting.isPending || rejecting.isPending,
    reread,
    accept: (id) =>
      accepting.mutate(id, {
        onSuccess: (draft) => {
          onDraft(draft);
          toast.success(t("workbench.conversation.applied"));
        },
        onError: refused,
      }),
    reject: (id) => rejecting.mutate(id, { onError: refused }),
  };
}
