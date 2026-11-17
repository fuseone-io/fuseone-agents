import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
import { AgentToolsSection } from "@/features/agents/agent-tools-section";
import { TabGovernance } from "@/features/agents/tab-governance";
import { agentTarget } from "@/features/agents/agent-target";
import { useTools } from "@/features/admin/api";
import { usePolicies } from "@/features/policies/api";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * The draft's fields, with the editor's own sections: what they change is a
 * local copy, and saving turns it into the draft's operations. The
 * identifier is shown and never edited: it was fixed at the draft's birth.
 */
export function WorkbenchFields({
  agentId,
  local,
  patch,
}: {
  agentId: string;
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const tools = useTools(local.company !== "", agentTarget(local));
  const policies = usePolicies();

  return (
    <>
      <AgentBasicsSection draft={local} patch={patch} agentId={agentId} editable={false} onAgentId={() => {}} />
      <AgentToolsSection
        catalogue={tools.data?.items ?? []}
        policies={policies.data?.items ?? []}
        granted={local.tools ?? []}
        patch={patch}
      />
      <TabGovernance draft={local} patch={patch} />
    </>
  );
}
