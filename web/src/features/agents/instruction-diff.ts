import { parse, type Block, type BlockKind } from "@/features/agents/instruction-blocks";
import { align } from "@/features/agents/align";

export interface Piece {
  kind: "same" | "added" | "removed";
  text: string;
}

export interface BlockDiff {
  kind: BlockKind;
  /** What happened to the block itself, which is a different fact from its words. */
  state: "same" | "added" | "removed" | "changed";
  pieces: Piece[];
}

/**
 * What publishing would change in the instruction, block by block.
 *
 * "Instructions changed" is the one line in the publish summary nobody can
 * check, and a character count does not distinguish a sentence tightened from
 * a rule deleted — which is the distinction a reviewer is there to make.
 *
 * Blocks are matched by what they say, not by where they sit. Aligned by
 * position, inserting one paragraph at the top reports every block below it as
 * rewritten, which tells the reviewer to read the whole text again instead of
 * the part that is new.
 */
export function diffInstructions(was: string, now: string): BlockDiff[] {
  const before = parse(was);
  const after = parse(now);
  const steps = align(before.map(keyOf), after.map(keyOf));

  const out: BlockDiff[] = [];
  for (let at = 0; at < steps.length; at++) {
    const step = steps[at]!;

    if (step.kind === "same") {
      out.push(unchanged(after[step.bi!]!));
      continue;
    }

    /*
    A block that left with one of the same kind arriving next to it is that
    block, edited. Reported as two — one deleted whole, one added whole — the
    reviewer has to diff it by eye, which is the work this exists to do.
    */
    const next = steps[at + 1];
    if (
      step.kind === "removed" &&
      next?.kind === "added" &&
      before[step.ai!]!.kind === after[next.bi!]!.kind
    ) {
      out.push(edited(before[step.ai!]!, after[next.bi!]!));
      at++;
      continue;
    }

    out.push(
      step.kind === "removed"
        ? whole(before[step.ai!]!, "removed")
        : whole(after[step.bi!]!, "added"),
    );
  }
  return out;
}

/** A block and its kind, which is what makes two of them the same block. */
function keyOf(block: Block): string {
  return `${block.kind}\n${block.text}`;
}

function unchanged(block: Block): BlockDiff {
  return { kind: block.kind, state: "same", pieces: [{ kind: "same", text: block.text }] };
}

function whole(block: Block, state: "added" | "removed"): BlockDiff {
  return { kind: block.kind, state, pieces: [{ kind: state, text: block.text }] };
}

function edited(before: Block, after: Block): BlockDiff {
  return { kind: after.kind, state: "changed", pieces: words(before.text, after.text) };
}

/**
 * The words that went and the words that came.
 *
 * Split on whitespace and keeping the whitespace, so what is rebuilt from the
 * pieces is the text and not an approximation of it — a diff that quietly
 * reflows the prose is a diff nobody can quote from.
 */
function words(before: string, after: string): Piece[] {
  const a = before.split(/(\s+)/);
  const b = after.split(/(\s+)/);

  const out: Piece[] = [];
  for (const step of align(a, b)) {
    const text = step.kind === "added" ? b[step.bi!]! : a[step.ai!]!;
    const last = out[out.length - 1];
    // Merged as they are produced: a run of five changed words is one mark to
    // read, and five adjacent marks is five things to look at.
    if (last?.kind === step.kind) last.text += text;
    else out.push({ kind: step.kind, text });
  }
  return out;
}
