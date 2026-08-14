import { ruleFor } from "@/features/agents/tool-rule";
import type { Policy, Tool } from "@/lib/api/client";

export type SegmentKind = "text" | "tool" | "denied" | "limit";

export interface Segment {
  kind: SegmentKind;
  text: string;
}

/**
 * What the prose talks about, found in the prose itself.
 *
 * Nothing is stored beside the text. A tool named in an instruction is the
 * bare identifier the model receives — the chip is a rendering of it — so
 * there is no span model that can drift from the words, and pasting a
 * paragraph from somewhere else lights up exactly as if it had been typed.
 *
 * Money is matched narrowly and deliberately: a currency mark and digits.
 * Guessing at "quinhentos reais" would mark a phrase somebody did not mean as
 * a limit, and a highlight that is wrong once is a highlight nobody reads
 * again.
 */
export function segments(
  text: string,
  catalogue: Tool[],
  policies: Policy[],
): Segment[] {
  const names = catalogue
    .map((tool) => tool.toolId)
    .sort((a, b) => b.length - a.length);

  // Longest first, so `crm.lookup` is never matched as `crm.look`.
  const pattern = new RegExp(
    [
      ...names.map(escape),
      // A currency mark, digits, and the separators a person writes — ending
      // on a digit, so the full stop that ends the sentence stays in the
      // sentence rather than becoming part of the amount.
      String.raw`(?:R\$|\$)\s?\d(?:[\d.,]*\d)?`,
    ].join("|"),
    "g",
  );

  const out: Segment[] = [];
  let at = 0;

  for (const found of text.matchAll(pattern)) {
    const start = found.index ?? 0;
    if (start > at) out.push({ kind: "text", text: text.slice(at, start) });

    const word = found[0];
    const tool = catalogue.find((one) => one.toolId === word);
    out.push({
      kind: tool ? kindOf(tool, policies) : "limit",
      text: word,
    });
    at = start + word.length;
  }

  if (at < text.length) out.push({ kind: "text", text: text.slice(at) });
  return out;
}

/**
 * Whether the prose is naming something the platform will refuse.
 *
 * A sentence that promises what a policy already denies is the one thing
 * worth marking in an instruction: it will not happen, the text says it will,
 * and the person who reads the text next is the one who finds out.
 */
function kindOf(tool: Tool, policies: Policy[]): SegmentKind {
  return ruleFor(tool.toolId, tool.effect, policies).kind === "blocked"
    ? "denied"
    : "tool";
}

function escape(name: string): string {
  return name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
