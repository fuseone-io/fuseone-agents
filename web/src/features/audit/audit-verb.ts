/**
 * What each verb reads as, and how much weight it carries.
 *
 * A trail where every row is coloured says nothing about where to look, so
 * only the decisions are: what the Gate refused, what it escalated, what a
 * person granted. A configuration change is recorded in full and rendered
 * quietly — it is context for the decisions, not a decision itself.
 */
export const VERBS: Record<string, { label: string; className: string }> = {
  "gate.allowed": { label: "permitiu", className: "text-success" },
  "gate.constrained": { label: "restringiu", className: "text-warning" },
  "gate.escalated": { label: "escalou", className: "text-warning" },
  "gate.blocked": { label: "bloqueou", className: "text-danger" },
  "gate.decided": { label: "decidiu", className: "text-muted-foreground" },
  "approval.granted": { label: "aprovou", className: "text-primary" },
  "approval.refused": { label: "recusou", className: "text-danger" },
  "tool.classified": {
    label: "classificou",
    className: "text-muted-foreground",
  },
  "provider.created": {
    label: "configurou provedor",
    className: "text-muted-foreground",
  },
  "provider.deleted": {
    label: "removeu provedor",
    className: "text-muted-foreground",
  },
  "server.created": {
    label: "configurou servidor",
    className: "text-muted-foreground",
  },
  "server.deleted": {
    label: "removeu servidor",
    className: "text-muted-foreground",
  },
  "budget.set": { label: "definiu teto", className: "text-muted-foreground" },
  "budget.cleared": {
    label: "removeu teto",
    className: "text-muted-foreground",
  },
  bootstrap_reopened: {
    label: "audit.reopened",
    className: "text-danger",
  },
};

/**
 * An unknown verb reads as itself rather than as a blank.
 *
 * The trail is append-only and outlives the console: an entry written by a
 * version that knew a verb this one does not must still be readable, and
 * showing nothing would be the console quietly editing history.
 */
export function verbOf(verb: string): { label: string; className: string } {
  return VERBS[verb] ?? { label: verb, className: "text-muted-foreground" };
}
