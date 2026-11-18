import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { MessagesSquare } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState, ErrorState } from "@/components/shared/states";
import { AuthoringToday } from "@/features/workbench/authoring-today";
import { WorkbenchPane } from "@/features/workbench/workbench-pane";
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
  const end = useRef<HTMLDivElement>(null);
  // A block body: the browser's scrollIntoView now returns a promise, and an
  // effect that returns anything but a clean-up function breaks the tree.
  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
  }, [items.length, composer.pending, proposals.data?.length]);

  return (
    <WorkbenchPane
      label={t("workbench.conversation.title")}
      header={
        <div className="flex items-center gap-2.5 px-4 py-3">
          <span className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <MessagesSquare className="size-3.5" aria-hidden />
          </span>
          <h2 className="text-sm font-medium">{t("workbench.conversation.title")}</h2>
          <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{t("workbench.conversation.hint")}</span>
          <AuthoringToday agentId={agentId} className="sm:hidden" />
          <ConversationErase agentId={agentId} disabled={items.length === 0} />
        </div>
      }
      footer={<ConversationComposer composer={composer} open={state === "open"} />}
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
        {/* The newest turn in view, as a conversation reads. */}
        <div ref={end} />
      </div>
    </WorkbenchPane>
  );
}
