import { AgentBudgetSection } from "@/features/agents/agent-budget-section";
import { AgentMemoryLearningSection } from "@/features/agents/agent-memory-learning-section";
import { AgentTriggersSection } from "@/features/agents/agent-triggers-section";
import { NarrativeCard } from "@/features/agents/narrative-card";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * Who fires it, how far it goes, and the reading somebody approves.
 *
 * In the order a reviewer asks the questions, and the review card last —
 * every sentence in it comes from a field above, so approving it is approving
 * the agent. It is also the only accent surface on the screen: accent means
 * "this is the thing you sign".
 *
 * It replaces the rail that used to restate the page beside it. A summary is
 * either this card or the diff popover, never both.
 */
export function TabGovernance({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  return (
    <>
      <AgentTriggersSection draft={draft} patch={patch} />
      <AgentBudgetSection draft={draft} patch={patch} />
      <AgentMemoryLearningSection draft={draft} patch={patch} />
      <NarrativeCard draft={draft} />
    </>
  );
}
