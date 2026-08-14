import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { useStepProposal } from "@/features/agents/use-step-proposal";
import type { AgentDefinition } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * Drawing a process: what is selected, and the three things that change it.
 *
 * Held apart from the panels because none of it is about layout. Dropping a
 * tool grants it, moving a card reorders the sequence, and reading the
 * instructions replaces the drawing — three decisions with consequences, none
 * of them a rendering concern.
 */
export function useStepDrawing(
  draft: AgentDefinition,
  patch: (over: Partial<AgentDefinition>) => void,
) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<number | undefined>(undefined);

  // Reading the instructions is its own hook: it spends money at the provider
  // and everything below costs nothing and is undone by doing it again.
  const { read, reading } = useStepProposal(draft.instructions, (proposed) => {
    patch({ steps: proposed });
    setSelected(undefined);
  });

  const steps = draft.steps ?? [];
  const pack = draft.tools ?? [];

  /*
  Dropping a tool creates a stage that reaches it, and grants it when the
  agent did not hold it.

  The same authority the tools section of this form already carries, in the
  place somebody is actually thinking about the process. What it must not be
  is quiet: the rail shows what every tool does, and this says the pack grew.
  */
  const insert = (tool: string, at: number) => {
    const next = [...steps];
    next.splice(Math.min(at, next.length), 0, {
      name: "",
      reaches: tool ? [tool] : [],
    });

    const granting = tool !== "" && !pack.includes(tool);
    patch({ steps: next, ...(granting ? { tools: [...pack, tool] } : {}) });
    if (granting) toast.info(t("agents.alsoGranted", { tool }));
    setSelected(Math.min(at, next.length - 1));
  };

  const reorder = (from: number, to: number) => {
    const next = [...steps];
    const [moved] = next.splice(from, 1);
    if (moved) next.splice(to, 0, moved);
    patch({ steps: next });
    setSelected(to);
  };

  const change = (over: Partial<AgentStep>) =>
    patch({
      steps: steps.map((step, i) => (i === selected ? { ...step, ...over } : step)),
    });

  const remove = () => {
    patch({ steps: steps.filter((_, i) => i !== selected) });
    setSelected(undefined);
  };

  return {
    steps,
    pack,
    selected,
    setSelected,
    insert,
    reorder,
    read,
    change,
    remove,
    reading,
  };
}
