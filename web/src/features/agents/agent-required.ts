import type { AgentDefinition } from "@/lib/api/client";

export type AgentRequirementID =
  | "identifier"
  | "name"
  | "area"
  | "provider"
  | "model"
  | "instructions"
  | "tools"
  | "budget";

export interface AgentRequirement {
  id: AgentRequirementID;
  labelKey: string;
  done: boolean;
}

const REQUIRED_AGENT_FIELDS: Omit<AgentRequirement, "done">[] = [
  { id: "identifier", labelKey: "agents.identifier" },
  { id: "name", labelKey: "agents.fieldName" },
  { id: "area", labelKey: "agents.fieldArea" },
  { id: "provider", labelKey: "agents.provider" },
  { id: "model", labelKey: "agents.fieldModel" },
  { id: "instructions", labelKey: "agents.fieldInstructions" },
  { id: "tools", labelKey: "agents.fieldTools" },
  { id: "budget", labelKey: "agents.fieldBudget" },
];

/**
 * The fields a publish cannot invent.
 *
 * This mirrors the server's spec validation, with name and model included
 * because the editor presents them as part of the identity someone approves.
 * The publish button, the "what is missing" copy, and the explicit required
 * markers all read this list, so the screen does not keep a separate notion of
 * what publish will refuse.
 */
export function agentRequirements(
  agentId: string,
  draft: AgentDefinition,
): AgentRequirement[] {
  return REQUIRED_AGENT_FIELDS.map((field) => ({
    ...field,
    done: agentRequirementIsDone(field.id, agentId, draft),
  }));
}

export function agentRequirementMarked(id: AgentRequirementID): boolean {
  return REQUIRED_AGENT_FIELDS.some((item) => item.id === id);
}

function agentRequirementIsDone(
  id: AgentRequirementID,
  agentId: string,
  draft: AgentDefinition,
): boolean {
  switch (id) {
    case "identifier":
      return agentId.trim() !== "";
    case "name":
      return draft.name.trim() !== "";
    case "area":
      return draft.area.trim() !== "";
    case "provider":
      return draft.provider.trim() !== "";
    case "model":
      return draft.model.trim() !== "";
    case "instructions":
      return draft.instructions.trim() !== "";
    case "tools":
      return (draft.tools ?? []).length > 0;
    case "budget":
      return (
        (draft.budget?.micros ?? 0) > 0 || (draft.budget?.steps ?? 0) > 0
      );
  }
}
