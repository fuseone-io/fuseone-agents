import {
  agentRequirements,
  type AgentRequirement,
} from "@/features/agents/agent-required";
import type { EditorTab } from "@/features/agents/editor-tabs";
import type { MCPUserCredential } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import type { Agent, AgentDefinition, AgentTrust, Tool } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type GuidedAgentStepID =
  | "identity"
  | "instructions"
  | "steps"
  | "tools"
  | "governance"
  | "publish"
  | "simulation"
  | "launch";

export interface GuidedAgentStep {
  id: GuidedAgentStepID;
  labelKey: string;
  bodyKey: string;
  bodyValues?: Record<string, unknown>;
  tab?: EditorTab;
  to?: string;
  done: boolean;
  optional?: boolean;
}

export interface GuidedAgentContext {
  agentId?: string;
  catalogue?: Tool[];
  recipes?: ServerRecipe[];
  credentials?: MCPUserCredential[];
  channels?: Channel[];
  trust?: AgentTrust;
  simulationTo?: string;
}

type Channel = components["schemas"]["Channel"];
type Scope = components["schemas"]["Scope"];

/**
 * The first-agent path as the editor can prove it.
 *
 * The publish requirements stay the source of truth for what blocks a version.
 * This layer only groups those fields into the sequence a new author follows,
 * then adds operational checks whose facts already exist elsewhere: tool
 * catalogue, personal credentials and channel conversations.
 */
export function guidedAgentSteps(
  requirements: AgentRequirement[],
  draft: AgentDefinition,
  context: GuidedAgentContext = {},
): GuidedAgentStep[] {
  const done = (ids: AgentRequirement["id"][]) =>
    ids.every((id) => requirements.find((item) => item.id === id)?.done);
  const hasTriggers = (draft.triggers ?? []).length > 0;
  const tools = toolStep(done(["tools"]), draft, context);
  const governance = governanceStep(done(["budget"]), draft, context);

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
      bodyKey: tools.bodyKey,
      bodyValues: tools.bodyValues,
      tab: "tools",
      done: tools.done,
    },
    {
      id: "governance",
      labelKey: "agents.guideGovernance",
      bodyKey: governance.bodyKey,
      bodyValues: governance.bodyValues,
      tab: "governance",
      done: governance.done,
    },
    {
      id: "simulation",
      labelKey: "agents.guideSimulation",
      bodyKey: hasTriggers
        ? "agents.guideSimulationHint"
        : "agents.guideSimulationManualHint",
      tab: "governance",
      done: false,
      optional: true,
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

export function publishedAgentGuideSteps(
  agent: Agent,
  instructions: string | undefined,
  context: GuidedAgentContext = {},
): GuidedAgentStep[] {
  const draft = definitionFromAgent(agent, instructions);
  const requirements = agentRequirements(agent.agentId, draft);
  const done = (ids: AgentRequirement["id"][]) =>
    ids.every((id) => requirements.find((item) => item.id === id)?.done);
  const tools = toolStep(done(["tools"]), draft, context);
  const governance = governanceStep(done(["budget"]), draft, context);
  const running = agent.paused === false && agent.retired !== true;

  return [
    {
      id: "tools",
      labelKey: "agents.guideTools",
      bodyKey: tools.bodyKey,
      bodyValues: tools.bodyValues,
      done: tools.done,
      to: `/agents/${agent.agentId}/edit`,
    },
    {
      id: "governance",
      labelKey: "agents.guideGovernance",
      bodyKey: governance.bodyKey,
      bodyValues: governance.bodyValues,
      done: governance.done,
      to: `/agents/${agent.agentId}/edit`,
    },
    {
      id: "simulation",
      labelKey: "agents.guideSimulation",
      bodyKey: "agents.guideSimulationPublishedHint",
      done: simulationEvidenceReady(context.trust),
      optional: true,
      to: context.simulationTo ?? `/agents/${agent.agentId}/simulate`,
    },
    {
      id: "launch",
      labelKey: "agents.guideLaunch",
      bodyKey: running ? "agents.guideLaunchHint" : "agents.guideLaunchPausedHint",
      done: running,
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

function simulationEvidenceReady(trust?: AgentTrust): boolean {
  return (
    trust?.evidence?.some(
      (item) => item.id === "simulation" && item.status === "good",
    ) ?? false
  );
}

function toolStep(
  requirementDone: boolean,
  draft: AgentDefinition,
  context: GuidedAgentContext,
): Pick<GuidedAgentStep, "done" | "bodyKey" | "bodyValues"> {
  if (!requirementDone) {
    return { done: false, bodyKey: "agents.guideToolsHint" };
  }

  const selected = draft.tools ?? [];
  const catalogue = context.catalogue;
  if (!catalogue) {
    return {
      done: true,
      bodyKey: "agents.guideToolsReadyHint",
      bodyValues: { count: selected.length },
    };
  }

  const byID = new Map(catalogue.map((tool) => [tool.toolId, tool]));
  const chosen = selected.map((id) => byID.get(id)).filter(isTool);
  const missing = selected.length - chosen.length;
  if (missing > 0) {
    return {
      done: false,
      bodyKey: "agents.guideToolsMissingHint",
      bodyValues: { count: missing },
    };
  }

  const unavailable = chosen.filter((tool) => tool.offered === false).length;
  if (unavailable > 0) {
    return {
      done: false,
      bodyKey: "agents.guideToolsUnavailableHint",
      bodyValues: { count: unavailable },
    };
  }

  const outsideSurface = chosen.filter((tool) => tool.onSurface === false).length;
  if (outsideSurface > 0) {
    return {
      done: false,
      bodyKey: "agents.guideToolsSurfaceHint",
      bodyValues: { count: outsideSurface },
    };
  }

  const unclassified = chosen.filter(
    (tool) => tool.effect === "unknown" || tool.stale === true,
  ).length;
  if (unclassified > 0) {
    return {
      done: false,
      bodyKey: "agents.guideToolsClassifyHint",
      bodyValues: { count: unclassified },
    };
  }

  const missingCredentials = missingPersonalCredentialServers(chosen, context);
  if (missingCredentials > 0) {
    return {
      done: false,
      bodyKey: "agents.guideToolsPersonalCredentialHint",
      bodyValues: { count: missingCredentials },
    };
  }

  return {
    done: true,
    bodyKey: "agents.guideToolsReadyHint",
    bodyValues: { count: selected.length },
  };
}

function governanceStep(
  budgetDone: boolean,
  draft: AgentDefinition,
  context: GuidedAgentContext,
): Pick<GuidedAgentStep, "done" | "bodyKey" | "bodyValues"> {
  const triggers = draft.triggers ?? [];
  if (triggers.length === 0) {
    return { done: budgetDone, bodyKey: "agents.guideGovernanceManualHint" };
  }

  if (!triggers.some((trigger) => trigger.type === "channel")) {
    return { done: budgetDone, bodyKey: "agents.guideGovernanceHint" };
  }

  if (!context.channels) {
    return {
      done: false,
      bodyKey: "agents.guideGovernanceChannelUnknownHint",
    };
  }

  const routes = startableConversations(draft, context);
  if (routes === 0) {
    return { done: false, bodyKey: "agents.guideGovernanceNoChannelHint" };
  }

  return {
    done: budgetDone,
    bodyKey: "agents.guideGovernanceChannelHint",
    bodyValues: { count: routes },
  };
}

function startableConversations(
  draft: AgentDefinition,
  context: GuidedAgentContext,
) {
  const agentId = context.agentId ?? "";
  return (context.channels ?? []).reduce((count, channel) => {
    if (!channel.enabled) return count;
    return (
      count +
      channel.conversations.filter((conversation) => {
        if (!conversation.enabled) return false;
        if (!sameScope(conversation.scope, draft.company, draft.area)) return false;
        const mode = conversation.mode ?? "mentions";
        if (mode === "mentions") return true;
        if (!agentId) return false;
        return (
          (mode === "watch" || mode === "both") &&
          conversation.agent === agentId
        );
      }).length
    );
  }, 0);
}

function sameScope(scope: Scope, company: string, area: string) {
  return scope.company === company && (scope.area ?? "") === area;
}

function missingPersonalCredentialServers(
  tools: Tool[],
  context: GuidedAgentContext,
) {
  if (!context.recipes || !context.credentials) return 0;
  const recipes = new Map(context.recipes.map((recipe) => [recipe.server, recipe]));
  const credentials = new Map(
    context.credentials.map((credential) => [credential.server, credential]),
  );
  const servers = new Set(tools.map((tool) => tool.server));
  let missing = 0;
  for (const server of servers) {
    const recipe = recipes.get(server);
    if (!recipe || !recipeIsRemote(recipe)) continue;
    if (!recipe.requiresPersonalCredential) continue;
    if (credentials.get(server)?.hasCredential === true) continue;
    missing += 1;
  }
  return missing;
}

function recipeIsRemote(recipe: ServerRecipe) {
  return recipe.transport === "http" || Boolean(recipe.url);
}

function isTool(tool: Tool | undefined): tool is Tool {
  return tool !== undefined;
}

function definitionFromAgent(
  agent: Agent,
  instructions: string | undefined,
): AgentDefinition {
  return {
    name: agent.name,
    company: agent.scope.company,
    area: agent.scope.area ?? "",
    provider: agent.provider,
    model: agent.model,
    effort: agent.effort,
    instructions: instructions ?? "",
    tools: agent.tools,
    budget: agent.budget,
    triggers: agent.triggers,
  };
}
