import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
import { AgentBudgetSection } from "@/features/agents/agent-budget-section";
import { AgentInstructionsField } from "@/features/agents/agent-instructions-field";
import { AgentToolsSection } from "@/features/agents/agent-tools-section";
import { AgentTriggersSection } from "@/features/agents/agent-triggers-section";
import { NarrativeCard } from "@/features/agents/narrative-card";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * The form, in the order somebody fills it.
 *
 * Identity and instructions, then what starts it, then what it may touch, then
 * the ceilings. The process is not a section of its own: it is one of two
 * views of the instructions, because an author who has just described their
 * process in words should not find an empty form underneath asking for it
 * again.
 */
export function AgentEditorForm({
  draft,
  patch,
  agentId,
  creating,
  onAgentId,
  catalogue,
  policies,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  agentId: string;
  creating: boolean;
  onAgentId: (id: string) => void;
  catalogue: Tool[];
  policies: Policy[];
}) {
  return (
    <div className="flex flex-col gap-4">
      <AgentBasicsSection
        draft={draft}
        patch={patch}
        agentId={agentId}
        editable={creating}
        onAgentId={onAgentId}
      />
      {/* The process, next: what the agent is told to do and how that reads
          as stages. The pack is below because a stage can only narrow it. */}
      <AgentInstructionsField
        draft={draft}
        patch={patch}
        catalogue={catalogue}
        policies={policies}
      />
      <AgentTriggersSection draft={draft} patch={patch} />
      <AgentToolsSection
        granted={draft.tools ?? []}
        catalogue={catalogue}
        policies={policies}
        patch={patch}
      />
      <AgentBudgetSection draft={draft} patch={patch} />

      {/* Last, deliberately: the author fills the form and then reads back
          what it amounts to. FU-08 asks for approval of the prose, not of the
          fields. */}
      <NarrativeCard draft={draft} />
    </div>
  );
}
