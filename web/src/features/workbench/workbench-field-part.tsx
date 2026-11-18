import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
import { AgentToolsSection } from "@/features/agents/agent-tools-section";
import { TabGovernance } from "@/features/agents/tab-governance";
import { agentTarget } from "@/features/agents/agent-target";
import { useTools } from "@/features/admin/api";
import { usePolicies } from "@/features/policies/api";
import type { AgentDefinition } from "@/lib/api/client";

/** The parts of the draft a person edits by hand, each in its own side panel. */
export type FieldPart = "identity" | "tools" | "governance";

/**
 * One part of the draft's manual controls, with the editor's own sections:
 * what they change is the panel's local copy, and saving turns it into the
 * draft's operations. The identifier is shown and never edited: it was fixed
 * at the draft's birth.
 */
export function WorkbenchFieldPart({
  part,
  agentId,
  local,
  patch,
}: {
  part: FieldPart;
  agentId: string;
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const tools = useTools(part === "tools" && local.company !== "", agentTarget(local));
  const policies = usePolicies();

  if (part === "identity") {
    return <AgentBasicsSection draft={local} patch={patch} agentId={agentId} editable={false} onAgentId={() => {}} />;
  }
  if (part === "tools") {
    return (
      <AgentToolsSection
        catalogue={tools.data?.items ?? []}
        policies={policies.data?.items ?? []}
        granted={local.tools ?? []}
        patch={patch}
      />
    );
  }
  return <TabGovernance draft={local} patch={patch} />;
}
