import type { Policy } from "@/lib/api/client";

/**
 * How each effect reads, and the surface it reads on.
 *
 * Colour never carries the meaning: the pill says the word. A reader who is
 * colour-blind, or looking at a printout of an audit, gets the same answer.
 */
export const EFFECTS: Record<string, { label: string; className: string }> = {
  allow: { label: "permitir", className: "bg-success-surface text-success" },
  escalate: { label: "escalar", className: "bg-warning-surface text-warning" },
  deny: { label: "negar", className: "bg-danger-surface text-danger" },
};

export function effectOf(policy: Policy): { label: string; className: string } {
  return (
    EFFECTS[policy.effect] ?? { label: policy.effect, className: "bg-muted" }
  );
}

/**
 * What state a rule is actually in.
 *
 * Three, not two. Off is a decision; watching is a rule that runs and changes
 * nothing; in force is the only one that stops anything. Collapsing any pair
 * would report a governance state the installation is not in.
 */
export function stateOf(policy: Policy): { label: string; className: string } {
  if (policy.enabled === false) {
    return { label: "desligada", className: "text-muted-foreground" };
  }
  if (policy.mode === "monitor") {
    return { label: "monitorando", className: "text-warning" };
  }
  return { label: "impondo", className: "text-success" };
}
