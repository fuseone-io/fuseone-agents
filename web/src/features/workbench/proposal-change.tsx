import type { ReactNode } from "react";
import type { TFunction } from "i18next";
import { fieldValue } from "@/features/workbench/proposal-value";
import { ProposalTextDiff } from "@/features/workbench/proposal-text-diff";
import type { DraftProposal } from "@/lib/api/client";

const BLOCK_KINDS = new Set(["insert_block", "replace_block", "edit_block", "remove_block", "move_block"]);

/**
 * What a proposal would change, read before it is accepted, or undefined when
 * it cannot be shown. A block's is the whole instructions, now against what
 * accepting it would leave as the server renders them, line by line: the
 * label a new or rewritten block takes in its locale, where it lands, which
 * block moves.
 * The server sends that only while the proposal can be accepted; without it,
 * nothing here guesses.
 */
export function proposalChange(proposal: DraftProposal, instructions: string, t: TFunction): ReactNode | undefined {
  const op = proposal.operation;
  if (!op) return undefined;
  if (BLOCK_KINDS.has(op.kind)) {
    return proposal.instructionsAfter !== undefined
      ? <ProposalTextDiff was={instructions} now={proposal.instructionsAfter} />
      : undefined;
  }
  return fieldValue(op, t);
}
