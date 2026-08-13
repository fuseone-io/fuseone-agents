import type { Policy } from "@/lib/api/client";

/**
 * What will actually happen when this agent calls this tool.
 *
 * The handoff makes this a per-tool setting on the agent. It is not one here,
 * and making it one would be the fourth place that decides whether a human is
 * asked — beside the built-in ladder, the authored policies, and the taint
 * check. Four sources of truth about the same question is how an operator ends
 * up unable to say why something was blocked.
 *
 * So it is derived: the ladder, then whatever policy covers this tool. The
 * screen reports the platform's answer instead of offering a second one, and
 * changing it means editing the policy that produced it — which has an owner,
 * a reason, and a code that appears in the trail.
 */
export interface ToolRule {
  kind: "allowed" | "asks" | "blocked";
  label: string;
  /** Interpolated into label when it needs them. */
  labelValues?: Record<string, unknown>;
  /** The policy that produced this, when one did rather than the ladder. */
  because?: string;
}

/** The built-in ladder, which holds wherever no policy covers the call. */
function ladder(effect: string): ToolRule {
  switch (effect) {
    case "read":
      return { kind: "allowed", label: "agents.ruleAllowed" };
    case "write":
      return { kind: "asks", label: "agents.ruleAsks" };
    default:
      return { kind: "blocked", label: "agents.ruleBlocked" };
  }
}

/**
 * The policies covering one tool, most restrictive first, with the built-in
 * ladder underneath.
 *
 * Only the scope is read here, never the conditions: a rule that fires when
 * `args.rows > 100` does something to *some* calls, and a column claiming it
 * happens on every one would be worse than saying nothing. Those are reported
 * as conditional.
 */
export function ruleFor(
  tool: string,
  effect: string,
  policies: Policy[],
): ToolRule {
  const covering = policies.filter(
    (p) =>
      p.enabled !== false && p.mode === "enforce" && covers(p, tool, effect),
  );

  const unconditional = covering.filter(
    (p) => (p.conditions ?? []).length === 0,
  );
  const conditional = covering.filter((p) => (p.conditions ?? []).length > 0);

  for (const policy of unconditional) {
    if (policy.effect === "deny") {
      return {
        kind: "blocked",
        label: "agents.ruleBlocked",
        because: policy.code,
      };
    }
  }
  for (const policy of unconditional) {
    if (policy.effect === "escalate") {
      return {
        kind: "asks",
        label: "agents.ruleAsks",
        because: policy.code,
      };
    }
  }
  // An explicit allow is the one thing that lowers the built-in floor.
  for (const policy of unconditional) {
    if (policy.effect === "allow") {
      return {
        kind: "allowed",
        label: "agents.ruleAllowed",
        because: policy.code,
      };
    }
  }

  const base = ladder(effect);
  if (conditional.length > 0) {
    return {
      ...base,
      // The composite is a key too, with the base sentence inside it. Built
      // here as a pair rather than as a string: this module has no React
      // context, so a sentence assembled here is a sentence in one language.
      label: "agents.ruleSometimes",
      labelValues: { rule: base.label },
      because: conditional[0]!.code,
    };
  }
  return base;
}

/** Whether a policy reaches this tool at all, before any condition. */
function covers(policy: Policy, tool: string, effect: string): boolean {
  const resource = policy.resource ?? "*";
  const matchesTool =
    resource === "*" ||
    (resource.endsWith("*")
      ? tool.startsWith(resource.slice(0, -1))
      : resource === tool);
  if (!matchesTool) return false;

  const effects = policy.effects ?? [];
  return effects.length === 0 || effects.includes(effect as never);
}
