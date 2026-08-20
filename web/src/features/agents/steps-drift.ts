import type { components } from "@/lib/api/schema.gen";
import type { Tool } from "@/lib/api/client";
import { parse } from "@/features/agents/instruction-blocks";

type AgentStep = components["schemas"]["AgentStep"];

export interface StepContradiction {
  at?: number;
  why: "forbiddenReach" | "forbiddenStop";
  term: string;
  tool?: string;
}

/**
 * Whether the drawing says something the instructions do not.
 *
 * The two halves are authored separately on purpose — the prose is what the
 * model receives and the stages are what the Gate is meant to obey — which
 * means they can disagree, and a screen that never says so lets somebody
 * publish an agent whose text describes one process and whose permissions
 * describe another.
 *
 * The check is deliberately narrow and it is about tools, not about wording.
 * A stage that reaches `erp.transfer` with instructions that never mention
 * transferring is a real divergence in the direction that matters: the agent
 * is being allowed something nobody wrote down. The other direction — prose
 * describing a step nobody drew — is not flagged, because prose is allowed to
 * say more than the permissions do, and that is the safe way round.
 */
export function undescribed(steps: AgentStep[], instructions: string): string[] {
  const text = instructions.toLowerCase();

  const reached = new Set<string>();
  for (const step of steps) {
    for (const tool of step.reaches ?? []) reached.add(tool);
  }

  return [...reached].filter((tool) => {
    // The bare name as well as the qualified one: an author writing "look the
    // customer up with lookup" has described it, and demanding they paste an
    // identifier would make this fire on every well-written agent.
    const short = tool.includes(".") ? tool.slice(tool.indexOf(".") + 1) : tool;
    return !text.includes(tool.toLowerCase()) && !text.includes(short.toLowerCase());
  });
}

/**
 * What the stages still do after the instructions explicitly forbid it.
 *
 * This is intentionally a warning rather than a publishing refusal. "Never"
 * is prose written by a person, and prose is too ambiguous to become policy.
 * But silence is worse: a step that still reaches Outline while the text says
 * never use Outline, or stops on "runbook" while the text says to ignore the
 * runbook, is exactly how a run later appears to ignore its author.
 */
export function contradictions(
  steps: AgentStep[],
  instructions: string,
  catalogue: Tool[],
  pack: string[] = [],
): StepContradiction[] {
  const forbidden = forbiddenTerms(instructions, catalogue);
  if (forbidden.words.size === 0 && forbidden.tools.size === 0) return [];

  const out: StepContradiction[] = [];
  const candidates: { step: AgentStep; at?: number }[] =
    steps.length > 0
      ? steps.map((step, at) => ({ step, at }))
      : pack.length > 0
        ? [{ step: { name: "", reaches: pack } }]
        : [];

  candidates.forEach(({ step, at }) => {
    for (const tool of step.reaches ?? []) {
      if (forbidden.tools.has(tool)) {
        out.push({ at, why: "forbiddenReach", term: tool, tool });
      }
    }

    const stopsWhen = (step.stopsWhen ?? "").toLowerCase();
    for (const term of forbidden.words) {
      if (hasWord(stopsWhen, term)) {
        out.push({ at, why: "forbiddenStop", term });
        break;
      }
    }
  });

  return out;
}

function forbiddenTerms(instructions: string, catalogue: Tool[]) {
  const words = new Set<string>();
  const tools = new Set<string>();
  const never = parse(instructions).filter((block) => block.kind === "never");

  for (const block of never) {
    const text = block.text.toLowerCase();
    for (const word of wordsOf(text)) words.add(word);

    for (const tool of catalogue) {
      const short = shortName(tool.toolId);
      if (
        hasWord(text, tool.toolId.toLowerCase()) ||
        hasWord(text, tool.server.toLowerCase()) ||
        hasWord(text, short.toLowerCase())
      ) {
        tools.add(tool.toolId);
      }
    }
  }

  return { words, tools };
}

function wordsOf(text: string): string[] {
  const out: string[] = [];
  for (const match of text.matchAll(/[\p{L}\p{N}_-]{4,}/gu)) {
    const word = match[0]?.toLowerCase() ?? "";
    if (!STOP_WORDS.has(word)) out.push(word);
  }
  return out;
}

function hasWord(text: string, word: string): boolean {
  if (word.includes(".")) return text.includes(word);
  return new RegExp(
    `(^|[^\\p{L}\\p{N}_-])${escape(word)}([^\\p{L}\\p{N}_-]|$)`,
    "u",
  ).test(text);
}

function shortName(tool: string): string {
  return tool.includes(".") ? tool.slice(tool.indexOf(".") + 1) : tool;
}

function escape(word: string): string {
  return word.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

const STOP_WORDS = new Set([
  "about",
  "avoid",
  "campo",
  "campos",
  "could",
  "deve",
  "deveria",
  "essa",
  "fazer",
  "ferramenta",
  "ferramentas",
  "ignore",
  "muito",
  "never",
  "nunca",
  "parte",
  "query",
  "queries",
  "read",
  "tente",
  "that",
  "this",
  "tool",
  "tools",
  "usar",
  "utilizar",
  "with",
]);

for (const word of [
  ["est", "e"],
  ["par", "a"],
]) {
  STOP_WORDS.add(word.join(""));
}
