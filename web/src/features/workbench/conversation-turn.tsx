import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/shared/mono";
import { ProposalCard, type Decide, type Reading } from "@/features/workbench/proposal-card";
import type { DraftProposal, DraftTurn } from "@/lib/api/client";
import { formatMicros } from "@/lib/format";

/** The model's side of a turn: its reply, or what happened instead. */
function Answer({ turn }: { turn: DraftTurn }) {
  const { t } = useTranslation();
  if (turn.state === "in_progress") return <Skeleton className="h-4 w-2/3" />;
  if (turn.state === "failed") return <p className="text-sm text-muted-foreground">{t("workbench.conversation.failed")}</p>;
  if (turn.state === "unknown") return <p className="text-sm text-muted-foreground">{t("workbench.conversation.unknown")}</p>;
  return turn.reply ? <p className="whitespace-pre-wrap wrap-anywhere text-sm">{turn.reply}</p> : null;
}

/**
 * What the answer carried that never became a card, kept on the turn: a reply
 * announcing it would otherwise read as if it were waiting below.
 */
function Discarded({ turn }: { turn: DraftTurn }) {
  const { t } = useTranslation();
  return (
    <>
      {turn.dropped ? (
        <p className="text-xs text-muted-foreground">{t("workbench.conversation.dropped", { count: turn.dropped })}</p>
      ) : null}
      {turn.stale ? (
        <p className="text-xs text-muted-foreground">{t("workbench.conversation.staleOnArrival", { count: turn.stale })}</p>
      ) : null}
    </>
  );
}

/**
 * One turn: what the person said, what the model answered, what it cost, and
 * the proposals it made, each to decide. A turn whose words were erased keeps
 * its figures; one whose result is unknown says what it keeps reserved.
 */
export function ConversationTurn({
  turn,
  proposals,
  reading,
  decide,
}: {
  turn: DraftTurn;
  proposals: DraftProposal[];
  reading: Reading;
  decide: Decide;
}) {
  const { t } = useTranslation();
  const cost = turn.keptMicros !== undefined
    ? t("workbench.conversation.kept", { cost: formatMicros(turn.keptMicros) })
    : formatMicros(turn.costMicros);

  return (
    <li className="flex flex-col gap-2">
      <div className="flex min-w-0 max-w-full flex-col gap-1 self-end rounded-lg bg-muted px-3 py-2 sm:max-w-[85%]">
        {turn.erased ? (
          <p className="text-sm text-muted-foreground">{t("workbench.conversation.erased")}</p>
        ) : (
          <p className="whitespace-pre-wrap wrap-anywhere text-sm">{turn.message}</p>
        )}
        {turn.pasted && <Badge variant="outline" className="self-start">{t("workbench.conversation.pasted")}</Badge>}
      </div>
      <Answer turn={turn} />
      <Discarded turn={turn} />
      <Mono dim className="text-xs">{cost}</Mono>
      {proposals.length > 0 && (
        <div className="flex flex-col gap-2">
          {proposals.map((p) => (
            <ProposalCard key={p.proposalId} proposal={p} reading={reading} decide={decide} />
          ))}
        </div>
      )}
    </li>
  );
}
