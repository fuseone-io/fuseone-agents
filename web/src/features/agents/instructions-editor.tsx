import { useState } from "react";
import { useTranslation } from "react-i18next";
import { FileText } from "lucide-react";
import { Section } from "@/features/policies/section";
import type { Policy, Tool } from "@/lib/api/client";
import { InstructionRow } from "@/features/agents/instruction-row";
import { AddBlock } from "@/features/agents/add-block";
import { InstructionsPayload } from "@/features/agents/instructions-payload";
import { InstructionsDiffView } from "@/features/agents/instructions-diff-view";
import {
  InstructionsViewTabs,
  type InstructionsView,
} from "@/features/agents/instructions-view-tabs";
import { split } from "@/features/agents/instruction-blocks";
import { InstructionsStrip } from "@/features/agents/instructions-strip";
import { summarise } from "@/features/agents/instructions-summary";
import { useInstructionBlocks } from "@/features/agents/use-instruction-blocks";
import { agentRequirementMarked } from "@/features/agents/agent-required";

/**
 * What the model is told, written as prose and read as the payload.
 *
 * Not a textarea, and not rich text either. A mono box treats the one piece of
 * prose on the screen as configuration; formatting that did not survive into
 * what is sent would be a visual lie. What is left is the structure a
 * well-written prompt already has — a purpose, how to act, when to stop — made
 * visible, and a way to see exactly what leaves.
 *
 * The blocks are a way of writing. What a version stores is the text, and the
 * two directions agree: what is written as blocks reads back as those blocks,
 * and an instruction written before any of this existed stays whole.
 */
export function InstructionsEditor({
  instructions,
  on,
  tools,
  tokens,
  was,
}: {
  instructions: string;
  on: {
    change: (instructions: string) => void;
    /** Granting a tool the text names and the agent does not hold. */
    enable: (tool: string) => void;
  };
  tools: { catalogue: Tool[]; policies: Policy[]; enabled: string[] };
  /** How large this is to the model that will read it, when it could say. */
  tokens?: number;
  /** The instruction as published, when there is a published one. */
  was?: string;
}) {
  const { t, i18n } = useTranslation();
  const [view, setView] = useState<InstructionsView>("write");
  const editing = useInstructionBlocks(instructions, on.change, tools);
  const { blocks, found, menu, setMenu, write } = editing;

  return (
    <Section
      icon={FileText}
      title={t("agents.instructions")}
      hint={t("agents.instructionsHint")}
      required={agentRequirementMarked("instructions")}
      action={
        <InstructionsViewTabs
          view={view}
          onChange={setView}
          changed={was !== undefined && was !== instructions}
        />
      }
    >
      {view === "read" ? (
        <InstructionsPayload instructions={instructions} />
      ) : view === "diff" ? (
        <InstructionsDiffView was={was ?? ""} now={instructions} />
      ) : (
        <div className="flex flex-col gap-0.5">
          {blocks.map((block, at) => (
            <InstructionRow
              key={at}
              at={at}
              block={block}
              tools={tools}
              findings={found.filter((one) => one.at === at)}
              on={{
                change: (text) =>
                  write(blocks.map((b, i) => (i === at ? { ...b, text } : b))),
                remove: () => write(blocks.filter((_, i) => i !== at)),
                keep: (tool) => editing.keep(at, tool),
                enable: on.enable,
                relabel: (kind) =>
                  write(blocks.map((b, i) => (i === at ? { ...b, kind } : b))),
                split: () =>
                  write(blocks.flatMap((b, i) => (i === at ? split(b) : [b]))),
                slash: () => setMenu({ at }),
                drag: editing.drag,
              }}
            />
          ))}

          <AddBlock
            open={menu !== undefined}
            onOpenChange={(open) => setMenu(open ? {} : undefined)}
            onAdd={(kind) => {
              // Typed `/` becomes the block it asked for: the slash is the
              // gesture and never reaches the payload.
              const next = blocks.map((one, i) =>
                i === menu?.at ? { ...one, text: one.text.replace(/\/$/, "") } : one,
              );
              write([...next, { kind, text: "" }]);
              setMenu(undefined);
            }}
            // Citing from here writes the `@` into the last block and lets
            // the row take it from there: one gesture, one implementation.
            onCite={() => {
              const at = Math.max(0, blocks.length - 1);
              const block = blocks[at];
              if (!block) return write([{ kind: "prose", text: "@" }]);
              write(
                blocks.map((one, i) =>
                  i === at ? { ...one, text: `${one.text}@` } : one,
                ),
              );
            }}
            locale={i18n.language}
          />

          <InstructionsStrip
            summary={summarise(blocks, tools.catalogue, tools.policies, instructions)}
            findings={found}
            tokens={tokens}
          />
        </div>
      )}
    </Section>
  );
}
