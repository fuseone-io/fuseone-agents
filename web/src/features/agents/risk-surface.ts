import type { Tool } from "@/lib/api/client";

/**
 * What this agent can touch, in words.
 *
 * The handoff asks for it on both modes, and the reason is that a list of tool
 * ids does not answer the question anybody actually has. "crm.reply,
 * erp.transfer" is a list; "can write to two systems and move money" is the
 * thing somebody approves or refuses.
 */
export function riskSurface(tools: string[], catalogue: Tool[]): string[] {
  if (tools.length === 0) {
    return ["agents.riskNothing"];
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

  const lines: string[] = [];
  if (byEffect.read > 0) {
    lines.push(
      `Lê de ${byEffect.read} ${plural(byEffect.read, "ferramenta", "ferramentas")}.`,
    );
  }
  if (byEffect.write > 0) {
    lines.push(
      `Altera estado em ${byEffect.write} ${plural(byEffect.write, "sistema", "sistemas")}.`,
    );
  }
  if (byEffect.destructive > 0) {
    lines.push(
      `Apaga ou substitui de forma difícil de desfazer em ${byEffect.destructive}.`,
    );
  }
  if (byEffect.financial > 0) {
    lines.push(`Move dinheiro em ${byEffect.financial}.`);
  }
  // An unclassified tool never executes, which is a fact worth saying rather
  // than leaving somebody to wonder why nothing happens.
  if (byEffect.unknown > 0) {
    lines.push(
      `${byEffect.unknown} sem classificação — não executam até o Curador classificar.`,
    );
  }
  if (bringsOutside > 0) {
    lines.push(
      `${bringsOutside} ${plural(bringsOutside, "traz", "trazem")} dado de fora, o que marca a execução.`,
    );
  }
  return lines;
}

function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}
