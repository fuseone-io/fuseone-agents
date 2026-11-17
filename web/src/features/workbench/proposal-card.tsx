import { useTranslation } from "react-i18next";
import { Check, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Mono } from "@/components/shared/mono";
import { proposalChange } from "@/features/workbench/proposal-change";
import type { DraftBlock, DraftProposal } from "@/lib/api/client";

/** Kinds only a person decides: the model proposing one is always refused. */
const HUMAN_ONLY = new Set(["set_identity", "set_standing_approvals"]);

/** What a card can do with its proposal, and whether a decision is on its way. */
export type Decide = { accept: (id: string) => void; reject: (id: string) => void; busy: boolean };

/**
 * One proposal of the model, for its author to decide. Nothing it shows is
 * applied until it is accepted; an invalid one is shown with its reason and
 * cannot be. Accepting sends the id alone: the server applies what it stored.
 */
export function ProposalCard({
  proposal,
  blocks,
  decide,
}: {
  proposal: DraftProposal;
  blocks: DraftBlock[];
  decide: Decide;
}) {
  const { t } = useTranslation();
  const kind = t(`workbench.proposalKind.${proposal.kind}`);
  // Offered only when what it would change is on the card: an operation
  // erased, or one that does not carry what its kind needs, is not.
  const change = proposal.operation ? proposalChange(proposal.operation, blocks, t) : undefined;
  const waiting = proposal.valid && !proposal.decision && change !== undefined;

  return (
    <article aria-label={kind} className="flex flex-col gap-2 rounded-lg border border-border bg-background p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{kind}</span>
        {!proposal.valid && <Badge variant="destructive">{t("workbench.conversation.invalid")}</Badge>}
        {proposal.decision && (
          <Badge variant="outline">{t(`workbench.conversation.decision.${proposal.decision}`)}</Badge>
        )}
      </div>
      {change ?? (
        <p className="text-xs text-muted-foreground">
          {t(proposal.operation ? "workbench.conversation.unshowable" : "workbench.conversation.proposalErased")}
        </p>
      )}
      {/* The validator's words are machine text; a human-only kind gets this
          console's own sentence, since that refusal is the rule, not a fault. */}
      {!proposal.valid && HUMAN_ONLY.has(proposal.kind) ? (
        <p className="text-xs text-muted-foreground">{t("workbench.conversation.humanOnly")}</p>
      ) : (
        proposal.refusal && <Mono dim className="text-xs">{proposal.refusal}</Mono>
      )}
      {waiting && (
        <div className="flex flex-wrap gap-2">
          {/* Not the primary colour: the panel's one primary action is saving,
              and a turn can carry several proposals at once. */}
          <Button size="sm" variant="secondary" onClick={() => decide.accept(proposal.proposalId)} disabled={decide.busy}>
            <Check className="size-4" aria-hidden />
            {t("workbench.conversation.accept")}
          </Button>
          <Button size="sm" variant="outline" onClick={() => decide.reject(proposal.proposalId)} disabled={decide.busy}>
            <X className="size-4" aria-hidden />
            {t("workbench.conversation.reject")}
          </Button>
        </div>
      )}
    </article>
  );
}
