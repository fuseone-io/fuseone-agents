import { useState } from "react";
import type { Policy, PolicyInput } from "@/lib/api/client";

/** A new rule starts watching, never enforcing. Two-step adoption is the
 *  product's position, and the default is where a position is actually made. */
export const BLANK: PolicyInput = {
  name: "",
  owner: "",
  reason: "",
  resource: "*",
  effects: [],
  reach: "installation",
  conditions: [],
  effect: "deny",
  mode: "monitor",
  enabled: true,
};

/** Everything the form edits, and whether it differs from what was loaded. */
export function usePolicyDraft(loaded?: Policy) {
  const [draft, setDraft] = useState<PolicyInput>(
    () => toInput(loaded) ?? BLANK,
  );
  const patch = (over: Partial<PolicyInput>) =>
    setDraft((d) => ({ ...d, ...over }));

  const original = toInput(loaded);
  const changes = original ? changesBetween(original, draft) : [];

  return { draft, patch, changes };
}

/** Strips a stored policy back to what the form owns. */
export function toInput(policy?: Policy): PolicyInput | undefined {
  if (!policy) return undefined;
  return {
    name: policy.name,
    owner: policy.owner ?? "",
    reason: policy.reason ?? "",
    resource: policy.resource ?? "*",
    effects: policy.effects ?? [],
    reach: policy.reach ?? "installation",
    scopes: policy.scopes,
    agents: policy.agents,
    conditions: policy.conditions ?? [],
    effect: policy.effect,
    mode: policy.mode,
    enabled: policy.enabled ?? true,
  };
}

export interface Change {
  field: string;
  from: string;
  to: string;
}

/**
 * What is about to change, field by field.
 *
 * The handoff puts this in the side rail because saving a rule is a governance
 * act: somebody should see that widening the reach is part of what they are
 * about to do, rather than discovering it from the runs it stops.
 */
export function changesBetween(
  before: PolicyInput,
  after: PolicyInput,
): Change[] {
  const changes: Change[] = [];
  const compare = (field: string, from: unknown, to: unknown) => {
    const left = render(from);
    const right = render(to);
    if (left !== right) changes.push({ field, from: left, to: right });
  };

  compare("nome", before.name, after.name);
  compare("dono", before.owner, after.owner);
  compare("motivo", before.reason, after.reason);
  compare("recurso", before.resource, after.resource);
  compare("efeitos cobertos", before.effects, after.effects);
  compare("alcance", before.reach, after.reach);
  compare("condições", before.conditions, after.conditions);
  compare("efeito", before.effect, after.effect);
  compare("aplicação", before.mode, after.mode);
  compare("ligada", before.enabled, after.enabled);
  return changes;
}

function render(value: unknown): string {
  if (value === undefined || value === null || value === "") return "—";
  if (Array.isArray(value)) {
    return value.length === 0 ? "—" : value.map((v) => render(v)).join(", ");
  }
  if (typeof value === "object") {
    const parts = Object.entries(value as Record<string, unknown>)
      .filter(([, v]) => v !== undefined && v !== "")
      .map(([k, v]) => `${k} ${String(v)}`);
    return parts.join(" ");
  }
  return String(value);
}
