import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { parse, serialise, type Block } from "@/features/agents/instruction-blocks";
import { findings } from "@/features/agents/instruction-lint";
import { useListReorder } from "@/features/agents/use-list-reorder";
import type { Policy, Tool } from "@/lib/api/client";

/**
 * The blocks somebody is editing, and the text that leaves.
 *
 * Derived from the text on every render they could not hold a block that
 * contributes nothing to it — a stage somebody just added and has not written
 * in yet — so adding one appeared to do nothing at all. The round trip also
 * fought the typist: parsing trims, and a trailing space or a blank line
 * vanished as it was typed.
 *
 * So parsing happens when the text arrives from somewhere else — a template, a
 * version loading, the assistant proposing — and not when this screen is the
 * one that changed it.
 */
export function useInstructionBlocks(
  instructions: string,
  onChange: (text: string) => void,
  tools: { catalogue: Tool[]; policies: Policy[]; enabled: string[] },
) {
  const { i18n } = useTranslation();
  const [blocks, setBlocks] = useState(() => parse(instructions));
  const ours = useRef(instructions);

  useEffect(() => {
    if (instructions !== ours.current) setBlocks(parse(instructions));
  }, [instructions]);

  const write = (next: Block[]) => {
    setBlocks(next);
    const text = serialise(next, i18n.language);
    ours.current = text;
    onChange(text);
  };

  // Answered per block and kept for as long as the screen is open. Nothing is
  // stored: "keep it, it explains" is a decision about this sentence now, and
  // a definition carrying a list of silenced warnings would be a second thing
  // to review beside the text.
  const [kept, setKept] = useState<string[]>([]);

  /*
  Who asked for the block menu: a block that had `/` typed in it, the button,
  or nobody.

  One state rather than two, because the menu is controlled and a controlled
  menu opens only when the state says so. Held as two — a slash index and the
  trigger's own — it opened for the slash and ignored the button, which is
  exactly what happened.
  */
  const [menu, setMenu] = useState<{ at?: number } | undefined>(undefined);

  const drag = useListReorder((from, to) => {
    const next = [...blocks];
    const [moved] = next.splice(from, 1);
    if (moved) next.splice(to, 0, moved);
    write(next);
  });

  const found = findings(
    blocks,
    tools.catalogue,
    tools.policies,
    tools.enabled,
  ).filter((one) => !kept.includes(`${one.at}:${one.tool}`));

  return {
    blocks,
    found,
    menu,
    setMenu,
    write,
    drag,
    keep: (at: number, tool: string) => setKept([...kept, `${at}:${tool}`]),
  };
}
