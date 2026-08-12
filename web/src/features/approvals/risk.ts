import type { components } from "@/lib/api/schema.gen";

export type Effect = components["schemas"]["Effect"];
export type Risk = "high" | "medium" | "low" | "unknown";

/**
 * How much is at stake, from what the tool does to the world.
 *
 * The design asks for a risk level. Inventing a scale nobody maintains would
 * put a number on a screen that means whatever the reader assumes; the
 * Curator's classification is the real thing, and it is already the basis on
 * which the Gate decided to stop and ask.
 */
export function riskOf(effect: Effect | undefined): Risk {
  switch (effect) {
    case "destructive":
    case "financial":
      return "high";
    case "write":
      return "medium";
    case "read":
      return "low";
    default:
      return "unknown";
  }
}

export const RISK_LABEL: Record<Risk, string> = {
  high: "Alto",
  medium: "cost.average",
  low: "Baixo",
  unknown: "Não classificado",
};

export const RISK_DOT: Record<Risk, string> = {
  high: "bg-danger",
  medium: "bg-warning",
  low: "bg-muted-foreground",
  unknown: "bg-muted-foreground",
};

export const EFFECT_LABEL: Record<Effect, string> = {
  read: "leitura",
  write: "escrita",
  destructive: "destrutivo",
  financial: "financeiro",
};
