import { useTranslation } from "react-i18next";
import { MessagesSquare } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState, ErrorState } from "@/components/shared/states";
import { Section } from "@/features/policies/section";
import { ConversationComposer } from "@/features/workbench/conversation-composer";
import { ConversationErase } from "@/features/workbench/conversation-erase";
import { ConversationTurn } from "@/features/workbench/conversation-turn";
import { settledOf, useProposals, useTurns } from "@/features/workbench/conversation-api";
import { useComposer } from "@/features/workbench/use-composer";
import { useDecide } from "@/features/workbench/use-decide";
import type { RevisedDraft } from "@/features/workbench/api";
import type { DraftProposal } from "@/lib/api/client";

/** The proposals of each turn, by the turn that made them. */
function byTurn(proposals: DraftProposal[] = []): Map<string, DraftProposal[]> {
  const out = new Map<string, DraftProposal[]>();
  for (const p of proposals) {
    const key = p.turnId ?? "";
    out.set(key, [...(out.get(key) ?? []), p]);
  }
  return out;
}

/**
 * The person's own conversation with the assistant over the draft (NT-018
 * phase 3). The model only proposes; a proposal changes the draft once its
 * author accepts it. Closed for the day, or with no assistant, it says why and
 * the rest of the panel stays as it is.
 */
export function WorkbenchConversation({
  agentId,
  instructions,
  onDraft,
}: {
  agentId: string;
  /** The draft's instructions now, which block proposals are read against. */
  instructions: string;
  onDraft: (draft: RevisedDraft) => void;
}) {
  const { t } = useTranslation();
  const turns = useTurns(agentId);
  const proposals = useProposals(agentId, settledOf(turns.data?.items));
  const composer = useComposer(agentId);
  const decide = useDecide(agentId, onDraft, () => void proposals.refetch());
  const items = turns.data?.items ?? [];
  const state = turns.data?.conversation;
  const grouped = byTurn(proposals.data);
  const reading = { instructions };

  return (
    <Section
      icon={MessagesSquare}
      title={t("workbench.conversation.title")}
      hint={t("workbench.conversation.hint")}
      action={<ConversationErase agentId={agentId} disabled={items.length === 0} />}
      flush
    >
      <div className="flex flex-col gap-4 p-4">
        {state && state !== "open" && (
          <Alert>
            <AlertTitle>{t(`workbench.conversation.closed.${state}`)}</AlertTitle>
            <AlertDescription>{t("workbench.conversation.closedHint")}</AlertDescription>
          </Alert>
        )}
        {turns.isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : turns.error ? (
          <ErrorState error={turns.error} onRetry={() => void turns.refetch()} />
        ) : items.length === 0 && !composer.pending ? (
          <EmptyState title={t("workbench.conversation.emptyTitle")} hint={t("workbench.conversation.emptyHint")} />
        ) : (
          <ol className="flex flex-col gap-6">
            {items.map((turn) => (
              <ConversationTurn key={turn.turnId} turn={turn} proposals={grouped.get(turn.turnId) ?? []} reading={reading} decide={decide} />
            ))}
            {composer.pending && (
              <li className="flex flex-col gap-2">
                <p className="self-end whitespace-pre-wrap wrap-anywhere rounded-lg bg-muted px-3 py-2 text-sm">{composer.pending}</p>
                <Skeleton className="h-4 w-2/3" />
              </li>
            )}
          </ol>
        )}
      </div>
      <ConversationComposer composer={composer} open={state === "open"} />
    </Section>
  );
}
