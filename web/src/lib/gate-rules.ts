/**
 * The Gate's `reason` is developer-facing English; `rule` is the stable key it
 * ships alongside. The console localises from the key, so the server never has
 * to know the reader's language and the trail still names what fired.
 */
export const GATE_RULES: Record<string, string> = {
  passed: "",
  capability: "gate.outOfPack",
  contract: "gate.badArgs",
  data_barrier: "gate.dataBarrier",
  taint: "gate.tainted",
  policy: "gate.policyRequires",
  budget: "gate.overCeiling",
  idempotency: "gate.duplicate",
  approval: "gate.humanCleared",
};

export function explainRule(rule: string | undefined): string {
  return (rule && GATE_RULES[rule]) || "";
}
