import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
import { AgentBudgetSection } from "@/features/agents/agent-budget-section";
import { AgentStepsSection } from "@/features/agents/agent-steps-section";
import { AgentToolsSection } from "@/features/agents/agent-tools-section";
import { AgentTriggersSection } from "@/features/agents/agent-triggers-section";
import { NarrativeCard } from "@/features/agents/narrative-card";
import type { AgentDefinition, Policy, Tool } from "@/lib/api/client";

/**
 * The form, in the order somebody fills it.
 *
 * Identity, then what starts it, then what it may touch, then how that is
 * narrowed step by step, then the ceilings. The steps come after the pack
 * because a step can only ever narrow it: choosing what a stage reaches from
 * tools the agent does not hold would be a permission that reads as granted
 * and is refused at the Gate.
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
      <AgentTriggersSection draft={draft} patch={patch} />
      <AgentToolsSection
        granted={draft.tools ?? []}
        catalogue={catalogue}
        policies={policies}
        patch={patch}
      />
      <AgentStepsSection draft={draft} patch={patch} />
      <AgentBudgetSection draft={draft} patch={patch} />

      {/* Last, deliberately: the author fills the form and then reads back
          what it amounts to. FU-08 asks for approval of the prose, not of the
          fields. */}
      <NarrativeCard draft={draft} />
    </div>
  );
}
