import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Eye, FileText, Pencil } from "lucide-react";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Section } from "@/features/policies/section";
import type { Policy, Tool } from "@/lib/api/client";
import { InstructionRow } from "@/features/agents/instruction-row";
import { AddBlock } from "@/features/agents/add-block";
import { InstructionsPayload } from "@/features/agents/instructions-payload";
import {
  parse,
  serialise,
  type Block,
} from "@/features/agents/instruction-blocks";
import { findings } from "@/features/agents/instruction-lint";

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
  onChange,
  tools,
}: {
  instructions: string;
  onChange: (instructions: string) => void;
  tools: { catalogue: Tool[]; policies: Policy[] };
}) {
  const { t, i18n } = useTranslation();
  const [view, setView] = useState<"write" | "read">("write");

  const blocks = useMemo(() => parse(instructions), [instructions]);
  const write = (next: Block[]) => onChange(serialise(next, i18n.language));

  // Answered per block and kept for as long as the screen is open. Nothing is
  // stored: "keep it, it explains" is a decision about this sentence now, and
  // a definition carrying a list of silenced warnings would be a second thing
  // to review beside the text.
  const [kept, setKept] = useState<string[]>([]);
  const found = findings(blocks, tools.catalogue, tools.policies).filter(
    (one) => !kept.includes(`${one.at}:${one.tool}`),
  );

  return (
    <Section
      icon={FileText}
      title={t("agents.instructions")}
      hint={t("agents.instructionsHint")}
      action={
        <div className="flex items-center gap-3">
          <Tabs value={view} onValueChange={(next) => setView(next as typeof view)}>
            <TabsList className="h-8">
              <TabsTrigger value="write">
                <Pencil className="size-3.5" aria-hidden />
                {t("agents.writeIt")}
              </TabsTrigger>
              <TabsTrigger value="read">
                <Eye className="size-3.5" aria-hidden />
                {t("agents.readAsAgent")}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
            {t("agents.instructionsLength", { count: instructions.trim().length })}
          </span>
        </div>
      }
    >
      {view === "read" ? (
        <InstructionsPayload instructions={instructions} />
      ) : (
        <div className="flex flex-col gap-0.5">
          {blocks.map((block, at) => (
            <InstructionRow
              key={at}
              block={block}
              onChange={(text) =>
                write(blocks.map((b, i) => (i === at ? { ...b, text } : b)))
              }
              onRemove={() => write(blocks.filter((_, i) => i !== at))}
              tools={tools}
              findings={found.filter((one) => one.at === at)}
              onKeep={(tool) => setKept([...kept, `${at}:${tool}`])}
            />
          ))}

          <AddBlock
            onAdd={(kind) => write([...blocks, { kind, text: "" }])}
            locale={i18n.language}
          />
        </div>
      )}
    </Section>
  );
}
