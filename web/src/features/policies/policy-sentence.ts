import type { PolicyInput } from "@/lib/api/client";

const OPERATORS: Record<string, string> = {
  eq: "=",
  ne: "≠",
  gt: ">",
  lt: "<",
  contains: "policies.contains",
  in: "policies.isIn",
};

const EFFECTS: Record<string, string> = {
  allow: "permitir",
  escalate: "escalar",
  deny: "negar",
};

/**
 * The rule as one line, while it is being written.
 *
 * A copy of what the server generates, and that duplication is the risk: two
 * renderings of the same structure will drift, and then the draft reads as one
 * rule and the stored one is another. It exists only because a draft has not
 * been sent anywhere yet and the author has to see what they are building.
 *
 * The moment a policy is saved, the screen shows the server's sentence. This
 * one is never displayed beside a stored rule.
 */
export function draftSentence(policy: PolicyInput): string {
  const parts = [policy.resource || "*"];

  if (policy.effects?.length) {
    parts.push(policy.effects.join(", "));
  }
  for (const condition of policy.conditions ?? []) {
    parts.push(
      `${condition.field} ${OPERATORS[condition.op] ?? condition.op} ${condition.value}`,
    );
  }

  let sentence = `${parts.join(" · ")} → ${EFFECTS[policy.effect] ?? policy.effect}`;
  if (policy.mode === "monitor") {
    sentence += " (apenas monitorando)";
  }
  return sentence;
}
