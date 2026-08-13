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
  allow: "verdict.allow",
  escalate: "verdict.require_approval",
  deny: "verdict.block",
};

/**
 * The rule as one line.
 *
 * The only renderer. The server used to compose this too, which meant two
 * renderings of one structure — the risk being that they drift, and the draft
 * then reads as one rule while the stored one is another. It also meant the
 * sentence arrived in whatever language the binary held, which was Portuguese
 * for every reader.
 *
 * So the server returns the fields the Gate evaluates and this composes the
 * line, in the place that has the words. A draft and a stored rule now read
 * identically because they are rendered by the same function.
 */
export function draftSentence(
  policy: PolicyInput,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  const parts = [policy.resource || "*"];

  if (policy.effects?.length) {
    parts.push(policy.effects.map((e) => t(`effect.${e}`)).join(", "));
  }
  for (const condition of policy.conditions ?? []) {
    const op = OPERATORS[condition.op] ?? condition.op;
    // An operator this console does not know is shown as it came: a rule
    // written by a later version must stay readable rather than blank.
    parts.push(
      `${condition.field} ${op.includes(".") ? t(op) : op} ${condition.value}`,
    );
  }

  const effect = EFFECTS[policy.effect];
  const sentence = `${parts.join(" · ")} → ${effect ? t(effect) : policy.effect}`;
  // Said in the sentence, not only in a badge elsewhere on the screen. A rule
  // that reads "→ deny" while denying nothing is the most misleading thing
  // this screen could show.
  return policy.mode === "monitor"
    ? `${sentence} ${t("policies.onlyMonitoring")}`
    : sentence;
}
