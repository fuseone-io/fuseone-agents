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
export function undescribed(
  steps: AgentStep[],
  instructions: string,
): string[] {
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
    return (
      !text.includes(tool.toLowerCase()) && !text.includes(short.toLowerCase())
    );
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

    const stopsWhen = canonicalText(step.stopsWhen ?? "");
    const stopTerm = forbiddenStopTerm(stopsWhen, forbidden);
    if (stopTerm) {
      out.push({ at, why: "forbiddenStop", term: stopTerm });
    }
  });

  return out;
}

function forbiddenTerms(instructions: string, catalogue: Tool[]) {
  const words = new Map<string, string>();
  const itemsOfWord = new Map<string, Set<number>>();
  const hammered = new Set<string>();
  const inShortItem = new Set<string>();
  const tools = new Set<string>();
  const never = parse(instructions).filter((block) => block.kind === "never");

  let itemIndex = 0;
  for (const block of never) {
    for (const item of itemsOf(block.text)) {
      const text = canonicalText(item);
      const seen = new Map<string, number>();
      for (const word of wordsOf(text)) {
        words.set(word, word);
        seen.set(word, (seen.get(word) ?? 0) + 1);
        let members = itemsOfWord.get(word);
        if (!members) itemsOfWord.set(word, (members = new Set()));
        members.add(itemIndex);
      }
      // A word the author hammers twice inside one prohibition is its
      // subject; a short prohibition makes every noun in it the subject.
      for (const [word, count] of seen) {
        if (count > 1) hammered.add(word);
        if (seen.size <= 6) inShortItem.add(word);
      }
      itemIndex += 1;
    }

    const text = canonicalText(block.text);
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

  return { words, itemsOfWord, hammered, inShortItem, tools };
}

/**
 * One prohibition at a time. Bullets are items; a block written as plain
 * prose is one item per line. The split is what lets a long item's
 * incidental nouns stay ordinary words instead of becoming a forbidden
 * vocabulary an operator has to write around.
 */
function itemsOf(text: string): string[] {
  const bullets = text
    .split(/\n(?=\s*[-*\u2022])/u)
    .flatMap((chunk) => (chunk.trim() ? [chunk] : []));
  if (bullets.length > 1) return bullets;
  return text.split("\n").flatMap((line) => (line.trim() ? [line] : []));
}

function wordsOf(text: string): string[] {
  const out: string[] = [];
  for (const match of text.matchAll(/[\p{L}\p{N}_-]{4,}/gu)) {
    const word = match[0]?.toLowerCase() ?? "";
    if (!STOP_WORDS.has(word)) out.push(word);
  }
  return out;
}

function forbiddenStopTerm(
  stopsWhen: string,
  forbidden: ReturnType<typeof forbiddenTerms>,
): string | undefined {
  if (missingInformationStop(stopsWhen)) return undefined;
  const matched = [...forbidden.words.keys()].filter((term) =>
    hasWord(stopsWhen, term),
  );
  if (matched.length === 0) return undefined;

  const strong = matched.find((term) => strongStopTerm(term, forbidden));
  if (strong) return strong;

  // Two matches only count together when one prohibition holds both: a stop
  // overlapping a single item is echoing it, while two lone nouns from two
  // long items is how an honest card gets rewritten three times.
  for (let i = 0; i < matched.length; i += 1) {
    const a = matched[i] ?? "";
    const items = forbidden.itemsOfWord.get(a) ?? new Set<number>();
    for (let j = i + 1; j < matched.length; j += 1) {
      const other =
        forbidden.itemsOfWord.get(matched[j] ?? "") ?? new Set<number>();
      if ([...items].some((item) => other.has(item))) return a;
    }
  }
  return undefined;
}

function strongStopTerm(
  term: string,
  forbidden: ReturnType<typeof forbiddenTerms>,
): boolean {
  if (/[_-]|\d/.test(term)) return true;
  if (forbidden.hammered.has(term)) return true;
  if ((forbidden.itemsOfWord.get(term)?.size ?? 0) >= 3) return true;
  return term.length >= 8 && forbidden.inShortItem.has(term);
}

function missingInformationStop(text: string): boolean {
  if (!MISSING_INFORMATION.test(text)) return false;
  return !EXTERNAL_ACTION.test(text);
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

function canonicalText(text: string): string {
  return text.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
}

const EXTERNAL_ACTION =
  /\b(call|chamar|consultar|create|criar|delete|deletar|enviar|export|exportar|fetch|ler|list|listar|query|read|update|usar|use|write)\b/u;

// `text` is already canonicalized before this runs, so accented Portuguese
// forms are intentionally listed without marks.
const MISSING_INFORMATION =
  /\b(cannot be found|faltam?|faltar|faltando|falta|incomplete|insufficient|lack|missing|nao houver|not found|sem (acesso|dados|informacao|informacoes|permissao|permissoes))\b/u;

const STOP_WORDS = new Set([
  "about",
  "analisar",
  "antes",
  "avoid",
  "campo",
  "campos",
  "check",
  "consultar",
  "could",
  "deve",
  "deveria",
  "enviar",
  "essa",
  "este",
  "executar",
  "exportar",
  "faca",
  "fazer",
  "ferramenta",
  "ferramentas",
  "houver",
  "ignore",
  "muito",
  "never",
  "nunca",
  "parte",
  "para",
  "prosseguir",
  "query",
  "queries",
  "read",
  "responda",
  "suficiente",
  "suficientes",
  "tente",
  "that",
  "this",
  "tool",
  "tools",
  "usar",
  "utilizar",
  "verificar",
  "verify",
  "with",
]);
