import { useTranslation } from "react-i18next";
import { Check, RefreshCw, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { EffectBadge } from "@/features/admin/effect-badge";
import { awaitsEffect } from "@/features/workbench/conversation-api";
import { proposalChange } from "@/features/workbench/proposal-change";
import type { DraftProposal } from "@/lib/api/client";

/** Kinds only a person decides: the model proposing one is always refused. */
const HUMAN_ONLY = new Set(["set_identity", "set_standing_approvals"]);

/** Why a proposal's change is not on its card. */
function unshown(proposal: DraftProposal): string {
  if (!proposal.operation) return "workbench.conversation.proposalErased";
  // A block proposal comes with its effect only while the draft is one it can
  // be accepted on: the one the model read, or one only its own turn's
  // acceptances moved. Without it, something else changed the draft since,
  // and accepting it would be refused.
  if (proposal.kind.endsWith("_block")) return "workbench.conversation.staleBlock";
  return "workbench.conversation.unshowable";
}

/** What a card reads its proposal against: the draft's instructions now. */
export type Reading = { instructions: string };


/** What a card can do with its proposal, whether a decision is on its way, and how to read the proposals again. */
export type Decide = { accept: (id: string) => void; reject: (id: string) => void; busy: boolean; reread: () => void };

/**
 * One proposal of the model, for its author to decide. Nothing it shows is
 * applied until it is accepted; an invalid one is shown with its reason and
 * cannot be. Accepting sends the id alone: the server applies what it stored.
 */
export function ProposalCard({
  proposal,
  reading,
  decide,
}: {
  proposal: DraftProposal;
  reading: Reading;
  decide: Decide;
}) {
  const { t } = useTranslation();
  const kind = t(`workbench.proposalKind.${proposal.kind}`);
  // Offered only when what it would change is on the card: an operation
  // erased, or one that does not carry what its kind needs, is not.
  const change = proposalChange(proposal, reading.instructions, t);
  // Ruled by the server in the draft's stored scope, the one accepting is
  // judged in: a tool proposal without it is not offered.
  const effect = proposal.toolEffect;
  const unruled = awaitsEffect(proposal);
  const pending = proposal.valid && !proposal.decision;
  const waiting = pending && change !== undefined && !unruled;

  return (
    <article aria-label={kind} className="flex flex-col gap-2 rounded-lg border border-border bg-background p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{kind}</span>
        {effect && <EffectBadge effect={effect} />}
        {!proposal.valid && <Badge variant="destructive">{t("workbench.conversation.invalid")}</Badge>}
        {proposal.decision && (
          <Badge variant="outline">{t(`workbench.conversation.decision.${proposal.decision}`)}</Badge>
        )}
      </div>
      {/* The model's words: shown as written, read as a claim to check. */}
      {proposal.reason && <p className="text-sm text-muted-foreground wrap-anywhere">{proposal.reason}</p>}
      {change ??
        ((pending || !proposal.operation) && (
          <p className="text-xs text-muted-foreground">{t(unshown(proposal))}</p>
        ))}
      {change !== undefined && unruled && (
        <p className="text-xs text-muted-foreground">{t("workbench.conversation.effectUnknown")}</p>
      )}
      {/* The validator's words are machine text; a human-only kind gets this
          console's own sentence, since that refusal is the rule, not a fault. */}
      {!proposal.valid && HUMAN_ONLY.has(proposal.kind) ? (
        <p className="text-xs text-muted-foreground">{t("workbench.conversation.humanOnly")}</p>
      ) : (
        proposal.refusal && <Mono dim className="text-xs">{proposal.refusal}</Mono>
      )}
      {/* Rejecting changes nothing in the draft, so it needs nothing shown;
          accepting needs the whole change on the card. */}
      {pending && (
        <div className="flex flex-wrap gap-2">
          {/* Not the primary colour: the panel's one primary action is saving,
              and a turn can carry several proposals at once. */}
          {waiting && (
            <Button size="sm" variant="secondary" onClick={() => decide.accept(proposal.proposalId)} disabled={decide.busy}>
              <Check className="size-4" aria-hidden />
              {t("workbench.conversation.accept")}
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={() => decide.reject(proposal.proposalId)} disabled={decide.busy}>
            <X className="size-4" aria-hidden />
            {t("workbench.conversation.reject")}
          </Button>
          {unruled && (
            <Button size="sm" variant="ghost" onClick={decide.reread}>
              <RefreshCw className="size-4" aria-hidden />
              {t("workbench.conversation.rereadEffect")}
            </Button>
          )}
        </div>
      )}
    </article>
  );
}
