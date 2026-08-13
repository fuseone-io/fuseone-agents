import type { Tool } from "@/lib/api/client";

/** One line of the surface: a key and the count it is about. */
export type RiskLine = { key: string; count: number };

/**
 * What this agent can touch, in words.
 *
 * The handoff asks for it on both modes, and the reason is that a list of tool
 * ids does not answer the question anybody actually has. "crm.reply,
 * erp.transfer" is a list; "can write to two systems and move money" is the
 * thing somebody approves or refuses.
 *
 * Keys and counts rather than sentences: the sentences were written here in
 * Portuguese and rendered straight into the page, so an installation running
 * in English read them anyway — and the empty case returned a key that nothing
 * translated, putting "agents.riskNothing" on screen.
 */
export function riskSurface(tools: string[], catalogue: Tool[]): RiskLine[] {
  if (tools.length === 0) {
    return [{ key: "agents.riskNothing", count: 0 }];
  }

  const effects = new Map(catalogue.map((t) => [t.toolId, t.effect]));
  const untrusted = new Set(
    catalogue.filter((t) => t.untrusted).map((t) => t.toolId),
  );

  const byEffect = {
    read: 0,
    write: 0,
    destructive: 0,
    financial: 0,
    unknown: 0,
  };
  let bringsOutside = 0;

  for (const tool of tools) {
    const effect = effects.get(tool) ?? "unknown";
    byEffect[effect as keyof typeof byEffect] += 1;
    if (untrusted.has(tool)) bringsOutside += 1;
  }

  const lines: RiskLine[] = [];
  for (const [effect, key] of [
    ["read", "agents.riskReads"],
    ["write", "agents.riskWrites"],
    ["destructive", "agents.riskDestroys"],
    ["financial", "agents.riskPays"],
    // An unclassified tool never executes, which is a fact worth saying rather
    // than leaving somebody to wonder why nothing happens.
    ["unknown", "agents.riskUnclassified"],
  ] as const) {
    const count = byEffect[effect];
    if (count > 0) lines.push({ key, count });
  }
  if (bringsOutside > 0) {
    lines.push({ key: "agents.riskFromOutside", count: bringsOutside });
  }
  return lines;
}
