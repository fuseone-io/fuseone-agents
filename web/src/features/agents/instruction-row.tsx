import { useState } from "react";
import {
  type Block,
  type BlockKind,
} from "@/features/agents/instruction-blocks";
import { BlockControls } from "@/features/agents/block-controls";
import { BlockLabel } from "@/features/agents/block-label";
import { withoutSentence } from "@/features/agents/without-sentence";
import { BlockText } from "@/features/agents/block-text";
import { InstructionFinding } from "@/features/agents/instruction-finding";
import type { Finding } from "@/features/agents/instruction-lint";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * One block: its label in the margin, its prose in the column.
 *
 * The label is a label and not a field — hierarchy every well-written prompt
 * already has, made visible so it can be moved. Prose is set in sans at a
 * readable measure; mono is for identifiers, and a whole field of it treats
 * what an auditor reads as configuration.
 */
export function InstructionRow({
  block,
  at,
  on,
  tools,
  findings,
}: {
  block: Block;
  /** Where this block sits, which is what a drop reorders against. */
  at: number;
  on: {
    change: (text: string) => void;
    remove: () => void;
    keep: (tool: string) => void;
    /** Granting a tool the text names and the agent does not hold. */
    enable: (tool: string) => void;
    /** Saying what this block is, which an older instruction never said. */
    relabel: (kind: BlockKind) => void;
    /** Breaking it at its blank lines, into parts that can be labelled. */
    split: () => void;
    /** `/` typed where a menu is wanted. */
    slash: () => void;
    /** Moving this block, which is moving it in the instruction. */
    drag: {
      onStart: (index: number) => void;
      onOver: (index: number) => void;
      onDrop: () => void;
    };
  };
  tools: { catalogue: Tool[]; policies: Policy[]; enabled?: string[] };
  findings: Finding[];
}) {
  const [writing, setWriting] = useState(false);
  const [citing, setCiting] = useState(false);

  /*
  `@` opens the catalogue, and picking one writes the identifier in place of
  it. The `@` never reaches the payload: it is the gesture, not the text.
  */
  const typed = (next: string) => {
    on.change(next);
    if (next.endsWith("@")) setCiting(true);
    // `/` at the start of a line, which is where somebody reaches for a menu
    // rather than a slash. Mid-sentence it is a slash and stays one.
    if (next.endsWith("/") && /(^|\n)\/$/.test(next)) on.slash();
  };

  const cite = (tool: string) => {
    on.change(`${block.text.replace(/@$/, "")}${tool}`);
    setCiting(false);
  };

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        on.drag.onOver(at);
      }}
      onDrop={(e) => {
        e.preventDefault();
        on.drag.onDrop();
      }}
      className="group grid min-w-0 grid-cols-[72px_minmax(0,1fr)_auto] items-start gap-x-3 rounded-md py-2.5 transition-colors hover:bg-surface-hover sm:grid-cols-[104px_minmax(0,1fr)_auto] sm:gap-x-5"
    >
      <BlockLabel kind={block.kind} onChange={on.relabel} />

      {/* Written in a textarea and read as prose, swapped on focus.

          The alternative is a contenteditable surface with chips inside it,
          which owns the caret, the paste and the undo stack — three things a
          browser already does correctly and a hand-written editor gets wrong
          on the day somebody pastes a paragraph from a document. What the
          chips are is a rendering, so nothing is lost by rendering them when
          nobody is typing. */}
      <BlockText
        block={block}
        writing={writing}
        onWriting={setWriting}
        tools={tools}
        typed={typed}
        cite={{ open: citing, onPick: cite, onClose: () => setCiting(false) }}
      />

      <BlockControls
        splittable={block.text.includes("\n\n")}
        onGrab={() => on.drag.onStart(at)}
        onDrop={on.drag.onDrop}
        onSplit={on.split}
        onRemove={on.remove}
      />

      {/* Outside the swap between writing and reading, because it belongs to
          the block rather than to a way of looking at it — and because a
          button inside the read pane disappears on the focus that clicking it
          causes, so the click lands on nothing. */}
      {findings.length > 0 && (
        <div className="col-start-2">
          {findings.map((finding) => (
            <InstructionFinding
              key={finding.tool}
              finding={finding}
              onRemove={() =>
                on.change(withoutSentence(block.text, finding.tool))
              }
              onKeep={() => on.keep(finding.tool)}
              onEnable={() => on.enable(finding.tool)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
