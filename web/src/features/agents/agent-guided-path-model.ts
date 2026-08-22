import type { AgentRequirement } from "@/features/agents/agent-required";
import type { EditorTab } from "@/features/agents/editor-tabs";
import type { AgentDefinition } from "@/lib/api/client";

export type GuidedAgentStepID =
  | "identity"
  | "instructions"
  | "steps"
  | "tools"
  | "governance"
  | "publish";

export interface GuidedAgentStep {
  id: GuidedAgentStepID;
  labelKey: string;
  bodyKey: string;
  tab: EditorTab;
  done: boolean;
  optional?: boolean;
}

/**
 * The first-agent path as the editor can prove it.
 *
 * The publish requirements stay the source of truth for what blocks a version.
 * This layer only groups those fields into the sequence a new author follows,
 * plus one recommended review of the stages that the server does not require.
 */
export function guidedAgentSteps(
  requirements: AgentRequirement[],
  draft: AgentDefinition,
): GuidedAgentStep[] {
  const done = (ids: AgentRequirement["id"][]) =>
    ids.every((id) => requirements.find((item) => item.id === id)?.done);
  const hasTriggers = (draft.triggers ?? []).length > 0;

  return [
    {
      id: "identity",
      labelKey: "agents.guideIdentity",
      bodyKey: "agents.guideIdentityHint",
      tab: "definition",
      done: done(["identifier", "name", "area", "provider", "model"]),
    },
    {
      id: "instructions",
      labelKey: "agents.guideInstructions",
      bodyKey: "agents.guideInstructionsHint",
      tab: "definition",
      done: done(["instructions"]),
    },
    {
      id: "steps",
      labelKey: "agents.guideSteps",
      bodyKey: "agents.guideStepsHint",
      tab: "steps",
      done: (draft.steps ?? []).length > 0,
      optional: true,
    },
    {
      id: "tools",
      labelKey: "agents.guideTools",
      bodyKey: "agents.guideToolsHint",
      tab: "tools",
      done: done(["tools"]),
    },
    {
      id: "governance",
      labelKey: "agents.guideGovernance",
      bodyKey: hasTriggers
        ? "agents.guideGovernanceHint"
        : "agents.guideGovernanceManualHint",
      tab: "governance",
      done: done(["budget"]),
    },
    {
      id: "publish",
      labelKey: "agents.guidePublish",
      bodyKey: "agents.guidePublishHint",
      tab: "governance",
      done: requirements.every((item) => item.done),
    },
  ];
}

export function guidedAgentProgress(steps: GuidedAgentStep[]) {
  const required = steps.filter((step) => !step.optional);
  return {
    done: required.filter((step) => step.done).length,
    total: required.length,
    next: steps.find((step) => !step.done),
  };
}
