export type BlockKind =
  | "prose"
  | "objective"
  | "howToAct"
  | "whenToStop"
  | "never"
  | "howToReply";

export interface Block {
  kind: BlockKind;
  text: string;
}

/**
 * The labels a block is written as, in every language this console speaks.
 *
 * The label is part of the payload — the model reads it — so it is written in
 * the language the author is writing in, and every language is recognised on
 * the way back. An author who writes in Portuguese and a colleague who reads
 * the console in English are looking at one definition, and it must not
 * become an unlabelled paragraph because the second one opened it.
 */
const LABELS: Record<Exclude<BlockKind, "prose">, string[]> = {
  objective: ["Objetivo", "Purpose"],
  howToAct: ["Como agir", "How to act"],
  whenToStop: ["Quando parar", "When to stop"],
  never: ["Nunca", "Never"],
  howToReply: ["Como responder", "How to reply"],
};

/** The order a block menu offers, and the order they read in. */
export const KINDS: Exclude<BlockKind, "prose">[] = [
  "objective",
  "howToAct",
  "whenToStop",
  "never",
  "howToReply",
];

/** What a kind is called, for the margin and for the payload. */
export function labelOf(kind: BlockKind, locale: string): string {
  if (kind === "prose") return "";
  const [pt, en] = LABELS[kind];
  return locale.startsWith("pt") ? (pt ?? "") : (en ?? "");
}

/**
 * Blocks, as one string.
 *
 * Label, newline, text, blank line between — deterministic and diffable, and
 * exactly what the model receives. A block with no label contributes only its
 * prose, which is what an instruction somebody pasted whole stays as.
 */
export function serialise(blocks: Block[], locale: string): string {
  return blocks
    .map((block) => {
      const text = block.text.trim();
      // A label with nothing under it is a heading the model reads and cannot
      // act on. The empty row stays on screen — somebody just added it — and
      // simply contributes nothing to what is sent.
      if (text === "") return "";
      const label = labelOf(block.kind, locale);
      return label === "" ? text : `${label}\n${text}`;
    })
    .filter((part) => part !== "")
    .join("\n\n");
}

/**
 * The same string, back as blocks.
 *
 * A line that is exactly a label, on its own, opens a block. Anything before
 * the first one is prose somebody wrote or pasted, and it stays whole: nobody
 * with an instruction already written is made to restructure it to save.
 */
export function parse(instructions: string): Block[] {
  const lines = instructions.split("\n");
  const blocks: Block[] = [];
  let current: Block = { kind: "prose", text: "" };

  const keep = () => {
    if (current.text.trim() !== "" || current.kind !== "prose") {
      blocks.push({ ...current, text: current.text.trim() });
    }
  };

  for (const line of lines) {
    const kind = kindOf(line.trim());
    if (kind) {
      keep();
      current = { kind, text: "" };
      continue;
    }
    current.text += current.text === "" ? line : `\n${line}`;
  }
  keep();

  return blocks;
}

function kindOf(line: string): Exclude<BlockKind, "prose"> | undefined {
  return KINDS.find((kind) =>
    LABELS[kind].some((label) => label.toLowerCase() === line.toLowerCase()),
  );
}
